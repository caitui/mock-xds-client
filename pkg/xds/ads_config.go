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
	"errors"
	"fmt"
	"log"
	"math/rand"
	"time"

	"github.com/caitui/mock-xds-client/pkg/istio"
	envoy_config_bootstrap_v3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	httpconnectionmanagerv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/golang/protobuf/proto"
)

type AdsConfig struct {
	APIType      envoy_config_core_v3.ApiConfigSource_ApiType
	Services     []*ServiceConfig
	Clusters     map[string]*ClusterConfig
	refreshDelay *time.Duration
	xdsInfo      istio.XdsInfo
	previousInfo *apiState
}

var _ istio.XdsStreamConfig = (*AdsConfig)(nil)

func (ads *AdsConfig) CreateXdsStreamClient() (istio.XdsStreamClient, error) {
	return NewAdsStreamClient(ads)
}

const defaultRefreshDelay = time.Second * 10

func (ads *AdsConfig) RefreshDelay() time.Duration {
	if ads.refreshDelay == nil {
		return defaultRefreshDelay
	}
	return *ads.refreshDelay
}

// InitAdsRequest creates a cds request
func (ads *AdsConfig) InitAdsRequest() interface{} {
	return CreateCdsRequest(ads)
}

func (ads *AdsConfig) Node() *envoy_config_core_v3.Node {
	return &envoy_config_core_v3.Node{
		Id:       ads.xdsInfo.ServiceNode,
		Cluster:  ads.xdsInfo.ServiceCluster,
		Metadata: ads.xdsInfo.Metadata,
	}
}

func (ads *AdsConfig) loadADSConfig(dynamicResources *envoy_config_bootstrap_v3.Bootstrap_DynamicResources) error {
	if dynamicResources == nil || dynamicResources.AdsConfig == nil {
		log.Println("DynamicResources is null")
		return errors.New("null point exception")
	}
	if err := dynamicResources.AdsConfig.Validate(); err != nil {
		log.Println("Invalid DynamicResources")
		return err
	}
	return ads.getAPISourceEndpoint(dynamicResources.AdsConfig)
}

func (ads *AdsConfig) getAPISourceEndpoint(source *envoy_config_core_v3.ApiConfigSource) error {
	if source.ApiType != envoy_config_core_v3.ApiConfigSource_GRPC {
		log.Printf("unsupported api type: %#v\n", source.ApiType)
		return errors.New("only support GRPC api type yet")
	}
	ads.APIType = source.ApiType
	if source.RefreshDelay == nil || source.RefreshDelay.GetSeconds() <= 0 {
		duration := defaultRefreshDelay
		ads.refreshDelay = &duration
	} else {
		duration := ConvertDuration(source.RefreshDelay)
		ads.refreshDelay = &duration
	}
	ads.Services = make([]*ServiceConfig, 0, len(source.GrpcServices))
	for _, service := range source.GrpcServices {
		t := service.TargetSpecifier
		target, ok := t.(*envoy_config_core_v3.GrpcService_EnvoyGrpc_)
		if !ok {
			continue
		}
		serviceConfig := ServiceConfig{}
		if service.Timeout == nil || (service.Timeout.GetSeconds() <= 0 && service.Timeout.GetNanos() <= 0) {
			duration := time.Duration(time.Second) // default connection timeout
			serviceConfig.Timeout = &duration
		} else {
			nanos := service.Timeout.Seconds*int64(time.Second) + int64(service.Timeout.Nanos)
			duration := time.Duration(nanos)
			serviceConfig.Timeout = &duration
		}
		clusterName := target.EnvoyGrpc.ClusterName
		serviceConfig.ClusterConfig = ads.Clusters[clusterName]
		if serviceConfig.ClusterConfig == nil {
			log.Printf("cluster not found: %s\n", clusterName)
			return fmt.Errorf("cluster not found: %s", clusterName)
		}
		ads.Services = append(ads.Services, &serviceConfig)
	}
	return nil
}

func (ads *AdsConfig) loadClusters(staticResources *envoy_config_bootstrap_v3.Bootstrap_StaticResources) error {
	if staticResources == nil {
		log.Println("StaticResources is null")
		err := errors.New("null point exception")
		return err
	}
	if err := staticResources.Validate(); err != nil {
		log.Println("Invalid StaticResources")
		return err
	}
	ads.Clusters = make(map[string]*ClusterConfig)
	for _, cluster := range staticResources.Clusters {
		name := cluster.Name
		config := ClusterConfig{}
		if cluster.TransportSocket != nil && cluster.TransportSocket.Name == wellknown.TransportSocketTls {
			config.TlsContext = cluster.TransportSocket
		}
		if cluster.LbPolicy != envoy_config_cluster_v3.Cluster_RANDOM {
			log.Println("only random lbPoliy supported, convert to random")
		}
		config.LbPolicy = envoy_config_cluster_v3.Cluster_RANDOM
		if cluster.ConnectTimeout.GetSeconds() <= 0 {
			duration := time.Second * 10
			config.ConnectTimeout = &duration // default connect timeout
		} else {
			duration := ConvertDuration(cluster.ConnectTimeout)
			config.ConnectTimeout = &duration
		}

		// TODO: can we ignore it?
		if len(cluster.LoadAssignment.Endpoints) == 0 {
			log.Println("xds v3 cluster.loadassignment is empty")
		}

		config.Address = make([]string, 0, len(cluster.LoadAssignment.GetEndpoints()[0].LbEndpoints))
		for _, host := range cluster.LoadAssignment.GetEndpoints()[0].LbEndpoints {
			endpoint := host.GetEndpoint()

			// Istio 1.8+ use istio-agent proxy request Istiod
			if pipe := endpoint.Address.GetPipe(); pipe != nil {
				newAddress := fmt.Sprintf("unix://%s", pipe.Path)
				config.Address = append(config.Address, newAddress)
				break
			}

			if endpoint.Address.GetSocketAddress() == nil {
				log.Println("xds v3 cluster.loadassignment pipe and socket both empty")
			}
			if port, ok := endpoint.Address.GetSocketAddress().PortSpecifier.(*envoy_config_core_v3.SocketAddress_PortValue); ok {
				newAddress := fmt.Sprintf("%s:%d", endpoint.Address.GetSocketAddress().Address, port.PortValue)
				config.Address = append(config.Address, newAddress)
			} else {
				log.Println("only PortValue supported")
				continue
			}
		}
		ads.Clusters[name] = &config
	}
	return nil
}

const connectionManager = "envoy.filters.network.http_connection_manager"

var (
	typeFactoryMapping = map[string]func() proto.Message{
		connectionManager: func() proto.Message { return new(httpconnectionmanagerv3.HttpConnectionManager) },
	}
)

// FIXME: does this datas will be overwrite the xds info?
func (ads *AdsConfig) loadStaticResources(staticResources *envoy_config_bootstrap_v3.Bootstrap_StaticResources) error {
	var clusters []*envoy_config_cluster_v3.Cluster
	if cs := staticResources.Clusters; cs != nil && len(cs) > 0 {
		clusters = make([]*envoy_config_cluster_v3.Cluster, 0, len(cs))
		for _, c := range cs {
			if name := c.Name; name == "zipkin" { // why ignore zipkin ?
				continue
			}
			clusters = append(clusters, c)
		}
	}

	return nil

}

// ServiceConfig for grpc service
type ServiceConfig struct {
	Timeout       *time.Duration
	ClusterConfig *ClusterConfig
}

// ClusterConfig contains a cluster info from static resources
type ClusterConfig struct {
	LbPolicy       envoy_config_cluster_v3.Cluster_LbPolicy
	Address        []string
	ConnectTimeout *time.Duration
	TlsContext     *envoy_config_core_v3.TransportSocket
}

// GetEndpoint return an endpoint address by random
func (c *ClusterConfig) GetEndpoint() (string, *time.Duration) {
	if c.LbPolicy != envoy_config_cluster_v3.Cluster_RANDOM || len(c.Address) < 1 {
		// never happen
		return "", nil
	}
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	idx := r.Intn(len(c.Address))

	return c.Address[idx], c.ConnectTimeout
}
