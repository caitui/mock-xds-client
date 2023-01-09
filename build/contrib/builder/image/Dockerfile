FROM centos:centos7 as builder

ENV CHROOT_MOSN_PREFIX         /ROOT/home/admin/xds-client
RUN useradd -ms /bin/bash admin

COPY ./xds-client $CHROOT_MOSN_PREFIX/bin/xds-client

RUN mkdir -p $MOSN_PREFIX/logs

RUN chmod +x $CHROOT_MOSN_PREFIX/bin/xds-client
RUN chown -R admin:admin /ROOT/home/admin


FROM centos:centos7
ENV TMP_FOLDER      /tmp

RUN yum install -y \
       ssh wget curl perl logrotate make build-essential procps \
       tsar tcpdump mpstat iostat vmstat sysstat \
       python-setuptools; yum clean all

# pip
WORKDIR $TMP_FOLDER
RUN wget https://mirrors.aliyun.com/pypi/packages/69/81/52b68d0a4de760a2f1979b0931ba7889202f302072cc7a0d614211bc7579/pip-18.0.tar.gz#sha256=a0e11645ee37c90b40c46d607070c4fd583e2cd46231b1c06e389c5e814eed76
RUN tar zvxf pip-18.0.tar.gz
WORKDIR $TMP_FOLDER/pip-18.0
RUN python setup.py install

RUN useradd -ms /bin/bash admin
# copy all in one layer
COPY --from=builder /ROOT /

ENTRYPOINT ["/home/admin/xds-client/bin/xds-client" , "-c" , "docker-desktop", "-w" , "10"]
