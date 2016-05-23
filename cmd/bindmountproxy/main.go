package main

import (
	"net/http"

	"github.com/csrwng/bindmountproxy/pkg/bindmountproxy"
)

func main() {
	cfg := defaultOpenShiftConfig()
	http.ListenAndServe(":2675", bindmountproxy.New(cfg))
}

func defaultOpenShiftConfig() *bindmountproxy.BindMountProxyConfig {
	imagePatterns := []string{
		"(openshift/origin$)|(openshift/origin:.*)",
		"openshift/origin-deployer.*",
		"openshift/origin-recycler.*",
		"openshift/origin-docker-builder.*",
		"openshift/origin-sti-builder.*",
		"openshift/origin-f5-router.*",
		"openshift/node.*",
	}

	cfg := &bindmountproxy.BindMountProxyConfig{}
	for _, pattern := range imagePatterns {
		cfg.BindMounts = append(cfg.BindMounts, bindmountproxy.ImageBindMountConfig{
			ImagePattern: pattern,
			Mounts: []bindmountproxy.BindMountConfig{
				{
					Source:      "/data/src/github.com/openshift/origin/_output/local/bin/linux/amd64/openshift",
					Destination: "/usr/bin/openshift",
				},
			},
		})
	}

	return cfg
}
