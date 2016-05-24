FROM centos:centos7

COPY ./bindmountproxy /usr/bin/proxy
CMD  /usr/bin/proxy
