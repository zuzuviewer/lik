package cmd

import (
	"encoding/json"
	"io"
	"log"
	"os"

	"github.com/spf13/cobra"
	"github.com/zuzuviewer/lik/internal"
	"gopkg.in/yaml.v3"
)

var (
	requestPath string
	namespace   string
	name        string
	output      string
)

func Run() {
	var rootCmd = &cobra.Command{
		Use:   "lik",
		Short: "Lik is a http client tool",
		RunE:  run,
	}
	rootCmd.PersistentFlags().StringVarP(&requestPath, "path", "p", "", "request file or directory")
	rootCmd.PersistentFlags().StringVar(&namespace, "namespace", "", "request namespace, if it is not empty, only requests in this namespace will do request")
	rootCmd.PersistentFlags().StringVarP(&name, "name", "n", "", "request name, if it is not empty, only request with this name will do request")
	rootCmd.PersistentFlags().StringVarP(&output, "output", "o", "", "request result writer destination")
	rootCmd.MarkFlagRequired("path")
	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}

func run(cmd *cobra.Command, args []string) error {
	out, err := getOutputWriter()
	if err != nil {
		return err
	}
	likConfig := readLikConfig()
	requests := parseRequestPath()
	if len(requests) == 0 {
		return nil
	}
	requestMap := formatRequests(requests)
	for ns, m := range requestMap {
		if namespace != "" && ns != namespace {
			continue
		}
		for _, request := range m {
			if !request.ShouldRequest(namespace, name) {
				continue
			}
			request.Do(likConfig, out)
		}
	}
	return nil
}

func parseRequestPath() []*internal.Request {
	info, err := os.Stat(requestPath)
	if err != nil {
		return nil
	}
	if info.IsDir() {
		return readRequestDirectory(requestPath)
	} else {
		return readRequestFile(requestPath)
	}
}

func readRequestDirectory(directory string) []*internal.Request {
	ret := make([]*internal.Request, 0)
	entry, err := os.ReadDir(directory)
	if err != nil {
		return ret
	}
	for _, e := range entry {
		if e.IsDir() {
			ret = append(ret, readRequestDirectory(directory+"/"+e.Name())...)
		} else {
			ret = append(ret, readRequestFile(directory+"/"+e.Name())...)
		}
	}
	return ret
}

func readRequestFile(filename string) []*internal.Request {
	ret := make([]*internal.Request, 0)
	b, err := os.ReadFile(filename)
	if err != nil {
		return ret
	}
	err = yaml.Unmarshal(b, &ret)
	if err == nil {
		return ret
	}
	json.Unmarshal(b, &ret)
	return ret
}

// key is namespace, value key is request name, value is request
func formatRequests(requests []*internal.Request) map[string]map[string]*internal.Request {
	var (
		ret = make(map[string]map[string]*internal.Request, 0)
	)
	for _, r := range requests {
		if m, ok := ret[r.Namespace]; ok {
			if _, exist := m[r.Name]; exist {
				log.Fatalf("request %s %s", r.Namespace, r.Name)
			} else {
				ret[r.Namespace][r.Name] = r
			}
		} else {
			ret[r.Namespace] = make(map[string]*internal.Request, 0)
			ret[r.Namespace][r.Name] = r
		}
	}
	return ret
}

func readLikConfig() *internal.LikConfig {
	ret := &internal.LikConfig{}
	configFile, err := os.Open("./config/lik.yaml")
	isYaml := true
	if err != nil {
		configFile, err = os.Open("./config/lik.json")
		if err != nil {
			return ret
		} else {
			isYaml = false
		}
	}
	b, err := io.ReadAll(configFile)
	if err != nil {
		return ret
	}
	if isYaml {
		yaml.Unmarshal(b, ret)
	} else {
		json.Unmarshal(b, ret)
	}
	return ret
}

func getOutputWriter() (io.Writer, error) {
	if output == "" {
		return os.Stdout, nil
	}
	return os.OpenFile(output, os.O_RDWR|os.O_APPEND|os.O_CREATE, os.ModePerm)
}
