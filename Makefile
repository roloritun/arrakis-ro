OUT_DIR := out
API_CLIENT_DIR := out/gen/serverapi
API_CLIENT_GO_PACKAGE_NAME := serverapi
CHV_API_DIR := out/gen/chvapi
CHV_API_GO_PACKAGE_NAME := chvapi
RESTSERVER_BIN := ${OUT_DIR}/arrakis-restserver
CLIENT_BIN := ${OUT_DIR}/arrakis-client
GUESTINIT_BIN := ${OUT_DIR}/arrakis-guestinit
ROOTFSMAKER_BIN := ${OUT_DIR}/arrakis-rootfsmaker
CMDSERVER_BIN := ${OUT_DIR}/arrakis-cmdserver
CMDCLIENT_BIN := ${OUT_DIR}/arrakis-cmdclient
NOVNCSERVER_BIN := ${OUT_DIR}/arrakis-novncserver
CDPSERVER_BIN := ${OUT_DIR}/arrakis-cdpserver
GUESTROOTFS_BIN := ${OUT_DIR}/arrakis-guestrootfs-ext4.img
VSOCKSERVER_BIN := ${OUT_DIR}/arrakis-vsockserver
VSOCKCLIENT_BIN := ${OUT_DIR}/arrakis-vsockclient
INITRAMFS_SRC_DIR := initramfs

.PHONY: all clean serverapi chvapi initramfs restserver client guestinit rootfsmaker cmdserver novncserver cdpserver guestrootfs guest vsockclient vsockserver

clean:
	rm -rf ${OUT_DIR}

all: serverapi chvapi restserver client guestinit rootfsmaker cmdserver novncserver cdpserver guestrootfs guest vsockclient vsockserver

serverapi: ${OUT_DIR}/arrakis-serverapi.stamp
${OUT_DIR}/arrakis-serverapi.stamp: ./api/server-api.yaml
	mkdir -p ${API_CLIENT_DIR}
	openapi-generator-cli generate -i $< -g go -o ${API_CLIENT_DIR} --package-name ${API_CLIENT_GO_PACKAGE_NAME} \
	--git-user-id abshkbh \
	--git-repo-id arrakis/${API_CLIENT_DIR} \
    --additional-properties=withGoMod=false \
	--global-property models,supportingFiles,apis,apiTests=false
	rm -rf openapitools.json

chvapi: ${OUT_DIR}/arrakis-chvapi.stamp
${OUT_DIR}/arrakis-chvapi.stamp: api/chv-api.yaml
	mkdir -p ${API_CLIENT_DIR}
	openapi-generator-cli generate -i ./api/chv-api.yaml -g go -o ${CHV_API_DIR} --package-name ${CHV_API_GO_PACKAGE_NAME} \
	--git-user-id abshkbh \
	--git-repo-id arrakis/${CHV_API_DIR} \
    --additional-properties=withGoMod=false \
	--global-property models,supportingFiles,apis,apiTests=false
	rm -rf openapitools.json

restserver: serverapi chvapi
	mkdir -p ${OUT_DIR}
	CGO_ENABLED=0 go build -o ${RESTSERVER_BIN} ./cmd/restserver

client: serverapi
	mkdir -p ${OUT_DIR}
	CGO_ENABLED=0 go build -o ${CLIENT_BIN} ./cmd/client

# Build the guest init binary explicitly statically if "os" or "net" are used by
# using the CGO_ENABLED=0 flag.
guestinit:
	mkdir -p ${OUT_DIR}
	CGO_ENABLED=0 go build -o ${GUESTINIT_BIN} ./cmd/guestinit

rootfsmaker:
	mkdir -p ${OUT_DIR}
	CGO_ENABLED=0 go build -o ${ROOTFSMAKER_BIN} ./cmd/rootfsmaker

cmdserver:
	mkdir -p ${OUT_DIR}
	CGO_ENABLED=0 go build -o ${CMDSERVER_BIN} ./cmd/cmdserver

novncserver:
	mkdir -p ${OUT_DIR}
	CGO_ENABLED=0 go build -o ${NOVNCSERVER_BIN} ./cmd/novncserver

cdpserver:
	mkdir -p ${OUT_DIR}
	CGO_ENABLED=0 go build -o ${CDPSERVER_BIN} ./cmd/cdpserver

guestrootfs: rootfsmaker initramfs cmdserver novncserver cdpserver vsockserver guestinit
	mkdir -p ${OUT_DIR}
	sudo ${OUT_DIR}/arrakis-rootfsmaker create -o ${GUESTROOTFS_BIN} -d ./resources/scripts/rootfs/Dockerfile

guest: guestinit rootfsmaker cmdserver novncserver cdpserver guestrootfs

vsockclient:
	mkdir -p ${OUT_DIR}
	go build -o ${VSOCKCLIENT_BIN} ./cmd/vsockclient

vsockserver:
	mkdir -p ${OUT_DIR}
	CGO_ENABLED=0 go build -o ${VSOCKSERVER_BIN} ./cmd/vsockserver

initramfs: ${OUT_DIR}/initramfs.stamp
${OUT_DIR}/initramfs.stamp: ${INITRAMFS_SRC_DIR}/create-initramfs.sh
	${INITRAMFS_SRC_DIR}/create-initramfs.sh
