FROM ubuntu:22.04

# Avoid prompts from apt.
ENV DEBIAN_FRONTEND=noninteractive

ARG USERNAME=elara
ARG PASSWORD=elara0000
RUN useradd -m $USERNAME && \
    echo "$USERNAME:$PASSWORD" | chpasswd && \
    adduser $USERNAME sudo

# Needed to see journal logs.
RUN usermod -aG adm elara

# Set timezone.
ENV TZ=Etc/UTC
RUN ln -snf /usr/share/zoneinfo/$TZ /etc/localtime && echo $TZ > /etc/timezone

# Packages required for Agent-S https://github.com/simular-ai/Agent-S.
ARG AGENT_S_PACKAGES="python3-tk gnome-screenshot cmake libcairo2-dev python3-gi python3-gi-cairo gir1.2-gtk-4.0 libgirepository1.0-dev gir1.2-atspi-2.0"

# Update and install common utilities.
RUN apt-get update && \
    apt-get install -y ${AGENT_S_PACKAGES} \
    init \
    systemd \
    ncat \
    bash \
    curl \
    wget \
    vim \
    nano \
    git \
    htop \
    net-tools \
    iputils-ping \
    iproute2 \
    traceroute \
    dnsutils \
    tcpdump \
    netcat-openbsd \
    ssh \
    sudo \
    man-db \
    less \
    procps \
    psmisc \
    lsof \
    rsync \
    tar \
    gzip \
    zip \
    unzip \
    ca-certificates \
    tzdata \
    tini \
    python3 \
    python3-venv \
    python3-pip \
    xvfb \
    xfce4 \
    xfce4-goodies \
    zsh \
    tigervnc-standalone-server \
    socat \
    strace \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

# To support sudo within the guest.
RUN chown root:root /usr/bin/sudo && chmod 4755 /usr/bin/sudo

# TODO: Tighten permissions on this directory after testing.
RUN mkdir -p /mnt/stateful && chmod 0777 /mnt/stateful

# Set up directory for the vsock server. This is required in case the overlayfs setup fails, we
# still need the vsockserver to be able to run.
RUN mkdir -p /tmp/vsockserver && chmod 0644 /tmp/vsockserver

# Add user binaries from the host into the guest rootfs in this section.
##############
RUN ln -s /usr/lib/systemd/system/multi-user.target /etc/systemd/system/default.target

ARG OUT_DIR=out
ARG RESOURCES_DIR=resources

ARG GUESTINIT_BIN=arrakis-guestinit
COPY ${OUT_DIR}/${GUESTINIT_BIN} /usr/local/bin/${GUESTINIT_BIN}
RUN chmod +x /usr/local/bin/${GUESTINIT_BIN}
COPY ${RESOURCES_DIR}/${GUESTINIT_BIN}.service /usr/lib/systemd/system/${GUESTINIT_BIN}.service
RUN ln -s /usr/lib/systemd/system/${GUESTINIT_BIN}.service /etc/systemd/system/multi-user.target.wants/${GUESTINIT_BIN}.service

ARG CMDSERVER_BIN=arrakis-cmdserver
COPY ${OUT_DIR}/${CMDSERVER_BIN} /usr/local/bin/${CMDSERVER_BIN}
RUN chmod +x /usr/local/bin/${CMDSERVER_BIN}
COPY ${RESOURCES_DIR}/${CMDSERVER_BIN}.service /usr/lib/systemd/system/${CMDSERVER_BIN}.service
RUN ln -s /usr/lib/systemd/system/${CMDSERVER_BIN}.service /etc/systemd/system/multi-user.target.wants/${CMDSERVER_BIN}.service

ARG VNCSERVER_BIN=arrakis-vncserver
COPY ${RESOURCES_DIR}/${VNCSERVER_BIN}.service /usr/lib/systemd/system/${VNCSERVER_BIN}.service
RUN ln -s /usr/lib/systemd/system/${VNCSERVER_BIN}.service /etc/systemd/system/multi-user.target.wants/${VNCSERVER_BIN}.service

ARG VSOCKSERVER_BIN=arrakis-vsockserver
COPY ${OUT_DIR}/${VSOCKSERVER_BIN} /usr/local/bin/${VSOCKSERVER_BIN}
RUN chmod +x /usr/local/bin/${VSOCKSERVER_BIN}
COPY ${RESOURCES_DIR}/${VSOCKSERVER_BIN}.service /usr/lib/systemd/system/${VSOCKSERVER_BIN}.service
RUN ln -s /usr/lib/systemd/system/${VSOCKSERVER_BIN}.service /etc/systemd/system/multi-user.target.wants/${VSOCKSERVER_BIN}.service

ARG NOVNCSERVER_BIN=arrakis-novncserver
COPY ${OUT_DIR}/${NOVNCSERVER_BIN} /usr/local/bin/${NOVNCSERVER_BIN}
RUN chmod +x /usr/local/bin/${NOVNCSERVER_BIN}
COPY ${RESOURCES_DIR}/${NOVNCSERVER_BIN}.service /usr/lib/systemd/system/${NOVNCSERVER_BIN}.service
RUN ln -s /usr/lib/systemd/system/${NOVNCSERVER_BIN}.service /etc/systemd/system/multi-user.target.wants/${NOVNCSERVER_BIN}.service

ARG CDPSERVER_BIN=arrakis-cdpserver
COPY ${OUT_DIR}/${CDPSERVER_BIN} /usr/local/bin/${CDPSERVER_BIN}
RUN chmod +x /usr/local/bin/${CDPSERVER_BIN}
COPY ${RESOURCES_DIR}/${CDPSERVER_BIN}.service /usr/lib/systemd/system/${CDPSERVER_BIN}.service
RUN ln -s /usr/lib/systemd/system/${CDPSERVER_BIN}.service /etc/systemd/system/multi-user.target.wants/${CDPSERVER_BIN}.service

# Prevent the renaming service that will change "eth0" to "ens*". If not done our init service
# inside the guest has race conditions while configuring the network.
RUN ln -s /dev/null /etc/systemd/network/99-default.link
##############

# Install Chrome.
RUN wget -O /tmp/google-chrome-stable_current_amd64.deb https://dl.google.com/linux/direct/google-chrome-stable_current_amd64.deb && \
    # Any failure here will be fixed by the `install -f` command below.
    dpkg -i /tmp/google-chrome-stable_current_amd64.deb || true && \
    apt-get update && apt-get install -f -y && \
    rm -rf /tmp/google-chrome-stable_current_amd64.deb && \
    apt-get clean && rm -rf /var/lib/apt/lists/*

# Install Node.js and npm from Ubuntu repositories
RUN apt-get update && \
    apt-get install -y nodejs npm && \
    apt-get clean && rm -rf /var/lib/apt/lists/*

# Install noVNC
RUN git clone https://github.com/novnc/noVNC.git /opt/novnc && \
    ln -s /opt/novnc/vnc.html /opt/novnc/index.html && \
    chmod +x /opt/novnc/utils/novnc_proxy && \
    # Create a simple wrapper script for noVNC
    echo '#!/bin/bash\ncd /opt/novnc && ./utils/novnc_proxy --vnc localhost:5901 --listen 6080' > /usr/local/bin/novnc && \
    chmod +x /usr/local/bin/novnc
