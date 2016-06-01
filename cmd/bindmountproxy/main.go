package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"

	"github.com/csrwng/bindmountproxy/pkg/bindmountproxy"
)

func main() {
	var cfg *bindmountproxy.BindMountProxyConfig
	flag.Parse()
	args := os.Args
	if len(args) < 2 {
		fmt.Printf(usage())
		os.Exit(1)
	}
	listenSpec := args[1]
	if len(os.Getenv("PROXY_CONFIG")) > 0 {
		configData, err := ioutil.ReadFile(os.Getenv("PROXY_CONFIG"))
		if err != nil {
			fmt.Printf("error: cannot read configuration: %v", err)
			os.Exit(1)
		}
		cfg := &bindmountproxy.BindMountProxyConfig{}
		err = json.Unmarshal(configData, cfg)
		if err != nil {
			fmt.Printf("error: cannot unmarshal configuration: %v", err)
			os.Exit(1)
		}
	} else {
		if len(args) < 3 {
			fmt.Printf("specify a path to the 'openshift' binary")
			os.Exit(1)
		}
		binariesPath := os.Args[2]
		cfg = defaultOpenShiftConfig(binariesPath)
	}
	err := http.ListenAndServe(listenSpec, bindmountproxy.New(cfg))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func defaultOpenShiftConfig(path string) *bindmountproxy.BindMountProxyConfig {
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
					Source:      path,
					Destination: "/usr/bin/openshift",
				},
			},
		})
	}

	return cfg
}

const usageString = `
Usage:
%[1]s LISTEN_SPEC OPENSHIFT_PATH

where LISTEN_SPEC is either a port (ie. :1080) 
or an IP and port (ie. 127.0.0.1:1080) 

and OPENSHIFT_PATH is the path to the openshift binary
(ie. /data/src/github.com/openshift/origin/_output/local/bin/linux/adm64/openshift )

Example:
%[1]s ":2375" $(which openshift)
`

func usage() string {
	return fmt.Sprintf(usageString, os.Args[0])
}
