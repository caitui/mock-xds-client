package config

import (
	"encoding/json"
)

const xdsConfigStr = `{
	"dynamic_resources": {
		"lds_config": {
			"ads": {}
		},
		"cds_config": {
			"ads": {}
		},
		"ads_config": {
			"api_type": "GRPC",
			"set_node_on_first_message_only": true,
			"transport_api_version": "V3",
			"grpc_services": [
				{
					"envoy_grpc": {
						"cluster_name": "xds-grpc"
					}
				}
			]
		}
	},
    "static_resources": {
		"clusters": [
		  {
			"name": "xds-grpc",
			"type" : "STATIC",
			"connect_timeout": "1s",
			"lb_policy": "ROUND_ROBIN",
			"load_assignment": {
			  "cluster_name": "xds-grpc",
			  "endpoints": [{
				"lb_endpoints": [{
				  "endpoint": {
					"address":{
					  "socket_address": {
						"address": "10.99.159.244",
                        "port_value": 15010
					  }
					}
				  }
				}]
			  }]
			},
			"circuit_breakers": {
			  "thresholds": [
				{
				  "priority": "DEFAULT",
				  "max_connections": 100000,
				  "max_pending_requests": 100000,
				  "max_requests": 100000
				},
				{
				  "priority": "HIGH",
				  "max_connections": 100000,
				  "max_pending_requests": 100000,
				  "max_requests": 100000
				}
			  ]
			},
			"upstream_connection_options": {
			  "tcp_keepalive": {
				"keepalive_time": 300
			  }
			},
			"max_requests_per_connection": 1,
			"http2_protocol_options": { }
		  }
	   ]
    }
}`

func NewMockXdsClientConfig() (*MockXdsClientConfig, error) {
	cfg := &MockXdsClientConfig{}
	if err := json.Unmarshal([]byte(xdsConfigStr), cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}
