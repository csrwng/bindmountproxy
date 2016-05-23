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

type ImageBindMountConfig struct {
	ImagePattern string            `json:"imagePattern"`
	Mounts       []BindMountConfig `json:"mounts"`
}

type BindMountProxyConfig struct {
	BindMounts []ImageBindMountConfig `json:"bindMounts"`
}

func New(config *BindMountProxyConfig) http.Handler {
	director := bindMountDirectorFunc(config)
	return dockerproxy.New(director)
}

type createContainerData struct {
	*docker.Config
	HostConfig *docker.HostConfig `json:"HostConfig,omitempty"`
}

func bindMountDirectorFunc(config *BindMountProxyConfig) func(*http.Request) {
	return func(req *http.Request) {
		if isContainerCreate(req) {
			body, err := ioutil.ReadAll(req.Body)
			if err != nil {
				// Log error
				return
			}
			bodyReader := bytes.NewBuffer(body)
			decoder := json.NewDecoder(bodyReader)
			data := &createContainerData{}
			err = decoder.Decode(data)
			if err != nil {
				glog.Errorf("Error decoding container create data: %v", err)
				bodyReader.Reset()
				req.Body = ioutil.NopCloser(bodyReader)
				return
			}
			err = addBindMounts(config, data)
			if err != nil {
				glog.Errorf("Error adding bind mounts: %v", err)
				bodyReader.Reset()
				req.Body = ioutil.NopCloser(bodyReader)
				return
			}
			newBody := &bytes.Buffer{}
			encoder := json.NewEncoder(newBody)
			encoder.Encode(data)

		}
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
					fmt.Sprintf("%s:%s", mount.Source, mount.Destination))
			}
		}
	}
	return nil
}

func isContainerCreate(req *http.Request) bool {
	return strings.HasSuffix(req.URL.Path, "/containers/create")
}
