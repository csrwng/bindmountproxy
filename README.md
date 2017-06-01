# Bindmount Proxy
  Proxy to bind mount binaries into Docker containers

## Use with 'oc cluster up'

1. Start a container with the proxy:

```
docker run --privileged --net=host \
       -v /var/run/docker.sock:/var/run/docker.sock 
       -d cewong/bindmountproxy proxy 127.0.0.1:2375 $(which openshift)
```

2. Set your DOCKER_HOST:

```
export DOCKER_HOST=tcp://127.0.0.1:2375
```

3. Start your cluster:

```
oc cluster up -e DOCKER_HOST=tcp://127.0.0.1:2375
```

## Custom Configuration

It is possible to specify a custom proxy configuration to automatically modify other images or mount
other files. The custom configuration can be specified with the `PROXY_CONFIG` environment variable.

An example configuration file is included in this directory as `example_config.json`

To specify a custom configuration, mount your custom config into the bindmount container and pass the 
`PROXY_CONFIG` environment variable:

```
docker run --privileged --net=host \ 
       -v /var/run/docker.sock:/var/run/docker.sock \
       -v ${HOME}/custom_config.json:/data/config.json \
       -e PROXY_CONFIG=/data/config.json \
       -d cewong/bindmountproxy proxy 127.0.0.1:2375
```
