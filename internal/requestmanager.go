package internal

import (
	"io"
	"log"
)

// RequestManager request manager
type RequestManager struct {
	namespace   string
	requestName string
	config      *LikConfig
	requests    []*Request
	output      io.Writer
}

func NewRequestManager(namespace, requestName string, config *LikConfig, requests []*Request, output io.Writer) *RequestManager {
	return &RequestManager{
		namespace:   namespace,
		requestName: requestName,
		config:      config,
		requests:    requests,
		output:      output,
	}
}

func (r *RequestManager) Run() error {
	if len(r.requests) == 0 {
		log.Printf("don't have any request")
		return nil
	}
	requestMap := formatRequests(r.requests)
	for ns, m := range requestMap {
		if r.namespace != "" && ns != r.namespace {
			continue
		}
		for _, request := range m {
			if !request.ShouldRequest(r.namespace, r.requestName) {
				continue
			}
			if err := request.Do(r.config, r.output); err != nil {
				if request.ExitOnFailure {
					return err
				}
			}
		}
	}
	return nil
}

// formatRequests key is namespace, value is requests
func formatRequests(requests []*Request) map[string][]*Request {
	var (
		ret = make(map[string][]*Request)
		// check already exist namespace+name
		records = make(map[string]struct{})
	)
	for _, r := range requests {
		if _, ok := ret[r.Namespace]; ok {
			if _, exist := records[r.Namespace+r.Name]; exist {
				log.Fatalf("request namespace '%s' name '%s' already exist", r.Namespace, r.Name)
			}
			records[r.Namespace+r.Name] = struct{}{}
		}
		ret[r.Namespace] = append(ret[r.Namespace], r)
	}
	return ret
}
