package mockclient

import (
	"context"
	"github.com/caitui/mock-xds-client/pkg/config"
	"github.com/caitui/mock-xds-client/pkg/istio"
	"github.com/caitui/mock-xds-client/pkg/utils"
	_struct "github.com/golang/protobuf/ptypes/struct"
	"log"
	"time"
)

type mockXdsClient struct {
	Ctx       context.Context
	Config    *config.MockXdsClientConfig
	xdsClient *istio.ADSClient
}

func newMockXdsClient(ctx context.Context, config *config.MockXdsClientConfig) *mockXdsClient {
	return &mockXdsClient{
		Ctx:    ctx,
		Config: config,
	}
}

// StartMockXdsClient returns a ADSClient, support some extensions on it.
func (m *mockXdsClient) startMockXdsClient() {
	c := m.Config

	log.Println("[client start] client start mock xds client")
	xdsClient, err := istio.NewAdsClient(c)
	if err != nil {
		log.Printf("start mock xds client failed: %#v\n", err)
		return
	} else {
		m.xdsClient = xdsClient
		// start xds client, sendRequestLoop receiveResponseLoop
		xdsClient.Start()

		// wait stop signal
		for {
			select {
			case <-m.Ctx.Done():
				// close xds client
				xdsClient.Stop()
				log.Println("xds client shutdown")
				return
			default:
				time.Sleep(1 * time.Second)
			}
		}
	}
}

func (m *mockXdsClient) stopMockXdsClient() {
	m.xdsClient.Stop()
}

func initXdsInfo(serviceCluster, serviceNode string) {
	// 从环境变量获取control plane类型
	istioType := utils.GetDefaultEnv("ISTIO_TYPE", istio.COMM_ISTIO_TYPE)
	// 获取service类型
	serviceType := utils.GetDefaultEnv("SERVICE_TYPE", istio.MULTI_SERVICE)
	// set value
	istio.SetServiceCluster(serviceCluster)
	istio.SetServiceNode(serviceNode)
	istio.SetMetadata(&_struct.Struct{
		Fields: map[string]*_struct.Value{
			"ISTIO_TYPE":   {Kind: &_struct.Value_StringValue{StringValue: istioType}},
			"SERVICE_TYPE": {Kind: &_struct.Value_StringValue{StringValue: serviceType}},
		},
	})
}

func runMockXdsClientInstance(ctx context.Context, cluster string) {

	// init xds info
	initXdsInfo(cluster, "127.0.0.1")

	conf, err := config.NewMockXdsClientConfig()
	if err != nil {
		log.Printf("Create new mock client config err: %+v\n", err)
		return
	}

	mockXdsClient := newMockXdsClient(ctx, conf)
	// 开启新的xds client，并阻塞等待结束信号
	mockXdsClient.startMockXdsClient()
}

func MockXdsClients(cluster string, clientCount int, stop chan struct{}) {

	ctx, cancel := context.WithCancel(context.Background())
	// 控制goroutine数量，维持恒定
	clientCtrl := make(chan struct{}, clientCount)

	for {
		select {
		case <-stop:
			close(clientCtrl)
			cancel()
			log.Println("mock xds clients shutdown")
			return
		case clientCtrl <- struct{}{}:
			// 当缓冲区没满时，开启新的mock xds client
			go func(ctx context.Context, cluster string) {
				// 阻塞，直到收到stop signal 或 goroutine退出
				runMockXdsClientInstance(ctx, cluster)
				// release chan buf
				<-clientCtrl
			}(ctx, cluster)
		}
	}
}
