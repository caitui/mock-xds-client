/*
 * Licensed to the Apache Software Foundation (ASF) under one or more
 * contributor license agreements.  See the NOTICE file distributed with
 * this work for additional information regarding copyright ownership.
 * The ASF licenses this file to You under the Apache License, Version 2.0
 * (the "License"); you may not use this file except in compliance with
 * the License.  You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package xds

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/caitui/mock-xds-client/pkg/istio"
	envoy_config_bootstrap_v3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/ptypes/duration"
)

func init() {
	istio.RegisterParseAdsConfig(UnmarshalResources)
}

func buildCloudInfo() istio.XdsInfo {
	// patch node
	xdsInfoPatch := istio.GetGlobalXdsInfo()
	// 根据istio类型构造不同的node
	istioType := xdsInfoPatch.Metadata.Fields["ISTIO_TYPE"].GetStringValue()
	// 根据service类型来模拟是N个服务  还是 1个服务N个pod
	serviceType := xdsInfoPatch.Metadata.Fields["SERVICE_TYPE"].GetStringValue()
	// service node
	appMockName := "demo-app"
	if serviceType == istio.MULTI_SERVICE {
		appMockName = RandomAppName()
	}

	podMockName := RandomPodName(appMockName) + "." + appMockName
	serviceNode := strings.Join([]string{"sidecar", RandomIp(), podMockName, appMockName}, "~")

	if istioType == istio.COMM_ISTIO_TYPE {
		xdsInfoPatch.ServiceNode = serviceNode
	} else if istioType == istio.SOFA_ISTIO_TPYE {
		// append env
		envTenant := fmt.Sprintf(
			"multitenancy.workspace=middleware~multitenancy.cluster=%s", xdsInfoPatch.ServiceCluster)

		xdsInfoPatch.ServiceNode = strings.Join([]string{serviceNode, envTenant}, "||")
	}

	// log
	log.Printf("mock service node: %s\n", xdsInfoPatch.ServiceNode)

	return xdsInfoPatch
}

// UnmarshalResources register  istio.ParseAdsConfig
func UnmarshalResources(dynamic, static json.RawMessage) (istio.XdsStreamConfig, error) {
	ads, err := unmarshalResources(dynamic, static)
	if err != nil {
		return nil, err
	}

	return ads, nil
}

// unmarshalResources used in order to convert bootstrap_v2 json to pb struct (go-control-plane), some fields must be exchanged format
func unmarshalResources(dynamic, static json.RawMessage) (*AdsConfig, error) {
	dynamicResources, err := unmarshalDynamic(dynamic)
	if err != nil {
		return nil, err
	}
	staticResources, err := unmarshalStatic(static)
	if err != nil {
		return nil, err
	}

	cfg := &AdsConfig{
		xdsInfo:      buildCloudInfo(),
		previousInfo: newApiState(),
	}
	// update static config to client config
	if err := cfg.loadClusters(staticResources); err != nil {
		return nil, err
	}
	if err := cfg.loadStaticResources(staticResources); err != nil {
		return nil, err
	}
	if err := cfg.loadADSConfig(dynamicResources); err != nil {
		return nil, err
	}
	return cfg, nil
}

func duration2String(duration *duration.Duration) string {
	d := time.Duration(duration.Seconds)*time.Second + time.Duration(duration.Nanos)*time.Nanosecond
	x := fmt.Sprintf("%.9f", d.Seconds())
	x = strings.TrimSuffix(x, "000")
	x = strings.TrimSuffix(x, "000")
	return x + "s"
}

func unmarshalDynamic(dynamic json.RawMessage) (*envoy_config_bootstrap_v3.Bootstrap_DynamicResources, error) {
	// no dynamic resource, returns nil error
	if len(dynamic) <= 0 {
		return nil, nil
	}
	dynamicResources := &envoy_config_bootstrap_v3.Bootstrap_DynamicResources{}
	resources := map[string]json.RawMessage{}
	if err := json.Unmarshal(dynamic, &resources); err != nil {
		log.Printf("fail to unmarshal dynamic_resources: %#v\n", err)
		return nil, err
	}
	adsConfigRaw, ok := resources["ads_config"]
	if !ok {
		log.Println("ads_config not found")
		return nil, errors.New("lack of ads_config")
	}
	adsConfig := map[string]json.RawMessage{}
	if err := json.Unmarshal([]byte(adsConfigRaw), &adsConfig); err != nil {
		log.Printf("fail to unmarshal ads_config: %#v\n", err)
		return nil, err
	}
	if refreshDelayRaw, ok := adsConfig["refresh_delay"]; ok {
		refreshDelay := duration.Duration{}
		if err := json.Unmarshal([]byte(refreshDelayRaw), &refreshDelay); err != nil {
			log.Printf("fail to unmarshal refresh_delay: %#v\n", err)
			return nil, err
		}

		d := duration2String(&refreshDelay)
		b, _ := json.Marshal(&d)
		adsConfig["refresh_delay"] = json.RawMessage(b)
	}
	b, err := json.Marshal(&adsConfig)
	if err != nil {
		log.Printf("fail to marshal refresh_delay: %#v\n", err)
		return nil, err
	}
	resources["ads_config"] = json.RawMessage(b)
	b, err = json.Marshal(&resources)
	if err != nil {
		log.Printf("fail to marshal ads_config: %#v\n", err)
		return nil, err
	}
	if err := jsonpb.UnmarshalString(string(b), dynamicResources); err != nil {
		log.Printf("fail to unmarshal dynamic_resources: %#v\n", err)
		return nil, err
	}
	if err := dynamicResources.Validate(); err != nil {
		log.Printf("invalid dynamic_resources: %#v\n", err)
		return nil, err
	}
	return dynamicResources, nil
}

func unmarshalStatic(static json.RawMessage) (*envoy_config_bootstrap_v3.Bootstrap_StaticResources, error) {
	if len(static) <= 0 {
		return nil, nil
	}
	staticResources := &envoy_config_bootstrap_v3.Bootstrap_StaticResources{}
	resources := map[string]json.RawMessage{}
	if err := json.Unmarshal([]byte(static), &resources); err != nil {
		log.Printf("fail to unmarshal static_resources: %#v\n", err)
		return nil, err
	}
	var data []byte
	if clustersRaw, ok := resources["clusters"]; ok {
		var clusters []json.RawMessage
		if err := json.Unmarshal([]byte(clustersRaw), &clusters); err != nil {
			log.Printf("fail to unmarshal clusters: %#v\n", err)
			return nil, err
		}
		for i, clusterRaw := range clusters {
			cluster := map[string]json.RawMessage{}
			if err := json.Unmarshal([]byte(clusterRaw), &cluster); err != nil {
				log.Printf("fail to unmarshal cluster: %#v\n", err)
				return nil, err
			}
			cb := envoy_config_cluster_v3.CircuitBreakers{}
			b, err := json.Marshal(&cb)
			if err != nil {
				log.Printf("fail to marshal circuit_breakers: %#v\n", err)
				return nil, err
			}
			cluster["circuit_breakers"] = json.RawMessage(b)
			b, err = json.Marshal(&cluster)
			if err != nil {
				log.Printf("fail to marshal cluster: %#v\n", err)
				return nil, err
			}
			clusters[i] = json.RawMessage(b)
		}
		b, err := json.Marshal(&clusters)
		if err != nil {
			log.Printf("fail to marshal clusters: %#v\n", err)
			return nil, err
		}
		data = b
	}
	resources["clusters"] = json.RawMessage(data)
	b, err := json.Marshal(&resources)
	if err != nil {
		log.Printf("fail to marshal resources: %#v\n", err)
		return nil, err
	}
	if err := jsonpb.UnmarshalString(string(b), staticResources); err != nil {
		log.Printf("fail to unmarshal static_resources: %#v\n", err)
		return nil, err
	}
	if err := staticResources.Validate(); err != nil {
		log.Printf("Invalid static_resources: %#v\n", err)
		return nil, err
	}
	return staticResources, nil
}
