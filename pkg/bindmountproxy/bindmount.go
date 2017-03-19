package bindmountproxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"regexp"
	"strings"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/golang/glog"

	"github.com/csrwng/bindmountproxy/pkg/dockerproxy"
)

type BindMountConfig struct {
	Source      string `json:"source"`
	Destination string `json:"destination"`
}

type EnvConfig struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type ImageBindMountConfig struct {
	ImagePattern string            `json:"imagePattern"`
	Mounts       []BindMountConfig `json:"mounts"`
	Env          []EnvConfig       `json:"env"`
}

type BindMountProxyConfig struct {
	BindMounts []ImageBindMountConfig `json:"bindMounts"`
}

func New(config *BindMountProxyConfig) http.Handler {
	requestModifier := bindMountRequestModifier(config)
	return dockerproxy.New(requestModifier)
}

type createContainerData struct {
	*docker.Config
	HostConfig *docker.HostConfig `json:"HostConfig,omitempty"`
}

func bindMountRequestModifier(config *BindMountProxyConfig) dockerproxy.RequestModifierFunc {
	return func(req *http.Request) (*http.Request, error) {
		if isContainerCreate(req) {
			body, err := ioutil.ReadAll(req.Body)
			if err != nil {
				// Log error
				return nil, err
			}
			decoder := json.NewDecoder(bytes.NewBuffer(body))
			data := &createContainerData{}
			err = decoder.Decode(data)
			if err != nil {
				glog.Errorf("Error decoding container create data: %v", err)
				return nil, err
			}
			err = addBindMounts(config, data)
			if err != nil {
				glog.Errorf("Error adding bind mounts: %v", err)
				return nil, err
			}

			newBody := &bytes.Buffer{}
			encoder := json.NewEncoder(newBody)
			encoder.Encode(data)
			newReq, err := http.NewRequest(req.Method, req.URL.String(), ioutil.NopCloser(newBody))
			if err != nil {
				return nil, err
			}
			newReq.Header = req.Header
			return newReq, nil
		}
		return req, nil
	}
}

func addBindMounts(config *BindMountProxyConfig, data *createContainerData) error {
	if config == nil {
		return nil
	}
	for _, imageConfig := range config.BindMounts {
		re, err := regexp.Compile(imageConfig.ImagePattern)
		if err != nil {
			// Log error
			return err
		}
		if re.MatchString(data.Image) {
			for _, mount := range imageConfig.Mounts {
				data.HostConfig.Binds = append(data.HostConfig.Binds,
					fmt.Sprintf("%s:%s:z", mount.Source, mount.Destination))
			}
			for _, env := range imageConfig.Env {
				data.Env = append(data.Env, fmt.Sprintf("%s=%s", env.Name, env.Value))
			}
		}
	}
	return nil
}

func isContainerCreate(req *http.Request) bool {
	return strings.HasSuffix(req.URL.Path, "/containers/create")
}

func isVersion(req *http.Request) bool {
	return strings.HasSuffix(req.URL.Path, "/version")
}
