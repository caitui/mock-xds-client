package config

import "encoding/json"

type MockXdsClientConfig struct {
	RawDynamicResources json.RawMessage `json:"dynamic_resources,omitempty"` //dynamic_resources raw message
	RawStaticResources  json.RawMessage `json:"static_resources,omitempty"`  //static_resources raw message
	Node                json.RawMessage `json:"node,omitempty"`              // node info for pilot
}
