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
    novnc \
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

# Copy guest configuration file
COPY ${RESOURCES_DIR}/guest-config.yaml /etc/config.yaml

ARG NOVNCSERVER_BIN=arrakis-novncserver
COPY ${OUT_DIR}/${NOVNCSERVER_BIN} /usr/local/bin/${NOVNCSERVER_BIN}
RUN chmod +x /usr/local/bin/${NOVNCSERVER_BIN}
COPY ${RESOURCES_DIR}/${NOVNCSERVER_BIN}.service /usr/lib/systemd/system/${NOVNCSERVER_BIN}.service
RUN ln -s /usr/lib/systemd/system/${NOVNCSERVER_BIN}.service /etc/systemd/system/multi-user.target.wants/${NOVNCSERVER_BIN}.service

# Add Chrome auto-start service
COPY ${RESOURCES_DIR}/arrakis-chrome.service /usr/lib/systemd/system/arrakis-chrome.service
RUN ln -s /usr/lib/systemd/system/arrakis-chrome.service /etc/systemd/system/multi-user.target.wants/arrakis-chrome.service

# Add Chrome CDP port forwarder service (forwards from 0.0.0.0:9223 to 127.0.0.1:9222)
COPY ${RESOURCES_DIR}/arrakis-chrome-forwarder.service /usr/lib/systemd/system/arrakis-chrome-forwarder.service
RUN ln -s /usr/lib/systemd/system/arrakis-chrome-forwarder.service /etc/systemd/system/multi-user.target.wants/arrakis-chrome-forwarder.service

# Prevent the renaming service that will change "eth0" to "ens*". If not done our init service
# inside the guest has race conditions while configuring the network.
RUN ln -s /dev/null /etc/systemd/network/99-default.link
##############

# Install Node.js from official binary distribution
RUN curl -fsSL https://nodejs.org/dist/v18.20.4/node-v18.20.4-linux-x64.tar.xz | tar -xJ -C /opt && \
    ln -s /opt/node-v18.20.4-linux-x64/bin/node /usr/local/bin/node && \
    ln -s /opt/node-v18.20.4-linux-x64/bin/npm /usr/local/bin/npm && \
    ln -s /opt/node-v18.20.4-linux-x64/bin/npx /usr/local/bin/npx

# Install Patchright (better than Playwright for AI automation) and Chromium
# Install system dependencies first, then install Patchright as elara user
RUN apt-get update && apt-get install -y \
    libnss3 \
    libatk-bridge2.0-0 \
    libdrm2 \
    libxkbcommon0 \
    libxcomposite1 \
    libxdamage1 \
    libxrandr2 \
    libgbm1 \
    libxss1 \
    libasound2 \
    && apt-get clean && rm -rf /var/lib/apt/lists/*

# Install Patchright as elara user
USER elara
WORKDIR /home/elara
RUN pip3 install --user patchright && \
    ~/.local/bin/patchright install chromium && \
    mkdir -p ~/.local/bin && \
    echo '#!/bin/bash' > ~/.local/bin/patchright-chrome && \
    echo 'CHROME_PATH=$(find ~/.cache/ms-playwright -name "chrome" -type f | head -1)' >> ~/.local/bin/patchright-chrome && \
    echo 'if [ -z "$CHROME_PATH" ]; then' >> ~/.local/bin/patchright-chrome && \
    echo '  echo "Chrome not found in ~/.cache/ms-playwright"' >> ~/.local/bin/patchright-chrome && \
    echo '  exit 1' >> ~/.local/bin/patchright-chrome && \
    echo 'fi' >> ~/.local/bin/patchright-chrome && \
    echo 'exec "$CHROME_PATH" "$@"' >> ~/.local/bin/patchright-chrome && \
    chmod +x ~/.local/bin/patchright-chrome

# Switch back to root for remaining setup
USER root
WORKDIR /

# Create system-wide symlinks for Chrome
RUN ln -sf /home/elara/.local/bin/patchright-chrome /usr/local/bin/patchright-chrome && \
    ln -sf /usr/local/bin/patchright-chrome /usr/local/bin/chromium-browser

# Install noVNC and websockify
RUN apt-get update && apt-get install -y python3-websockify && \
    git clone https://github.com/novnc/noVNC.git /opt/novnc && \
    ln -s /opt/novnc/vnc.html /opt/novnc/index.html && \
    # Create noVNC service script
    echo '#!/bin/bash\ncd /opt/novnc && python3 -m websockify.websockify 6080 localhost:5901' > /usr/local/bin/start-novnc && \
    chmod +x /usr/local/bin/start-novnc && \
    apt-get clean && rm -rf /var/lib/apt/lists/*
