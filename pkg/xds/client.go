package xds

import (
	"context"
	"errors"
	"log"
	"math"
	"path/filepath"
	"strings"

	"github.com/caitui/mock-xds-client/pkg/istio"
	envoy_service_discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	resource "github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	"google.golang.org/grpc"
)

type streamClient struct {
	client envoy_service_discovery_v3.AggregatedDiscoveryService_StreamAggregatedResourcesClient
	conn   *grpc.ClientConn
	cancel context.CancelFunc
}

func (sc *streamClient) Send(req *envoy_service_discovery_v3.DiscoveryRequest) error {
	if req == nil {
		return nil
	}
	return sc.client.Send(req)
}

type AdsStreamClient struct {
	streamClient *streamClient
	config       *AdsConfig
}

var _ istio.XdsStreamClient = (*AdsStreamClient)(nil)

func NewAdsStreamClient(c *AdsConfig) (*AdsStreamClient, error) {
	if len(c.Services) == 0 {
		log.Println("no available ads service")
		return nil, errors.New("no available ads service")
	}

	var endpoint string
	for _, service := range c.Services {
		if service.ClusterConfig == nil {
			continue
		}
		endpoint, _ = service.ClusterConfig.GetEndpoint()
		if len(endpoint) > 0 {
			break
		}
	}
	if len(endpoint) == 0 {
		log.Println("no available ads endpoint")
		return nil, errors.New("no available ads endpoint")
	}
	endpoint = normalizeUnixSocksPath(endpoint)
	sc := &streamClient{}

	conn, err := grpc.Dial(endpoint, grpc.WithInsecure(), generateDialOption())
	if err != nil {
		log.Printf("xds client grpc dial error: %s\n", err)
		return nil, err
	}
	log.Printf("client estab grpc connection to pilot at %s\n", endpoint)
	sc.conn = conn

	client := envoy_service_discovery_v3.NewAggregatedDiscoveryServiceClient(sc.conn)
	ctx, cancel := context.WithCancel(context.Background())
	sc.cancel = cancel
	streamClient, err := client.StreamAggregatedResources(ctx)
	if err != nil {
		log.Printf("fail to create stream client: %#v\n", err)
		if sc.conn != nil {
			_ = sc.conn.Close()
		}
		return nil, err
	}
	sc.client = streamClient
	return &AdsStreamClient{
		streamClient: sc,
		config:       c,
	}, nil
}

const (
	unixSocksPrefix       = "unix://"
	unixSocksPrefixLength = len(unixSocksPrefix)
)

func normalizeUnixSocksPath(maybeUnixSocks string) (normalized string) {
	if !strings.HasPrefix(maybeUnixSocks, unixSocksPrefix) {
		normalized = maybeUnixSocks
		return
	}
	absolutePath, _ := filepath.Abs(maybeUnixSocks[unixSocksPrefixLength:])
	normalized = unixSocksPrefix + absolutePath
	return
}

func (ads *AdsStreamClient) Send(req interface{}) error {
	if ads == nil {
		return errors.New("stream client is nil")
	}
	dr, ok := req.(*envoy_service_discovery_v3.DiscoveryRequest)
	if !ok {
		return errors.New("invalid request type")
	}
	return ads.streamClient.Send(dr)
}

func (ads *AdsStreamClient) Recv() (interface{}, error) {
	return ads.streamClient.client.Recv()
}

const (
	EnvoyListener = resource.ListenerType
	EnvoyCluster  = resource.ClusterType
	EnvoyEndpoint = resource.EndpointType
	EnvoyRoute    = resource.RouteType
)

func (ads *AdsStreamClient) HandleResponse(resp interface{}) {
	dresp, ok := resp.(*envoy_service_discovery_v3.DiscoveryResponse)
	if !ok {
		log.Println("invalid response type")
		return
	}
	// TODO: optimise the efficiency of xDS.
	// If xDS resource too big, Istio maybe have written timeout error when use sync, such as:
	// 2020-12-01T09:17:29.354132Z info ads Timeout writing sidecar~10.49.18.38
	var err error
	// Ads Handler, overwrite the AdsStreamClient to extends more ads or handle
	switch dresp.TypeUrl {
	case EnvoyCluster:
		err = ads.handleCds(dresp)
	case EnvoyEndpoint:
		err = ads.handleEds(dresp)
	case EnvoyListener:
		err = ads.handleLds(dresp)
	case EnvoyRoute:
		err = ads.handleRds(dresp)
	default:
		err = errors.New("unsupported type url")
	}
	if err != nil {
		log.Printf("handle typeurl %s failed, error: %#v\n", dresp.TypeUrl, err)
	}
}

func (ads *AdsStreamClient) AckResponse(resp *envoy_service_discovery_v3.DiscoveryResponse) {

	info := ads.config.previousInfo.Find(resp.TypeUrl)

	ack := &envoy_service_discovery_v3.DiscoveryRequest{
		VersionInfo:   resp.VersionInfo,
		ResourceNames: info.ResourceNames, // TODO: check it.
		TypeUrl:       resp.TypeUrl,
		ResponseNonce: resp.Nonce,
		ErrorDetail:   nil,
		Node:          ads.config.Node(),
	}
	if err := ads.streamClient.Send(ack); err != nil {
		log.Printf("response %s ack failed, error: %#v\n", resp.TypeUrl, err)
	}

}

func (ads *AdsStreamClient) Stop() {
	ads.streamClient.cancel()
	if ads.streamClient.conn != nil {
		ads.streamClient.conn.Close()
		ads.streamClient.conn = nil
	}
	ads.streamClient.client = nil
}

// [xds] [ads client] get resp timeout: rpc error: code = ResourceExhausted desc = grpc: received message larger than max (5193322 vs. 4194304), retry after 1s
// https://github.com/istio/istio/blob/9686754643d0939c1f4dd0ee20443c51183f3589/pilot/pkg/bootstrap/server.go#L662
// Istio xDS DiscoveryServer not set grpc MaxSendMsgSize. If this is not set, gRPC uses the default `math.MaxInt32`.
func generateDialOption() grpc.DialOption {
	return grpc.WithDefaultCallOptions(
		grpc.MaxCallRecvMsgSize(math.MaxInt32),
	)
}
