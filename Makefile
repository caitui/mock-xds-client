SHELL = /bin/bash

export GO111MODULE="on"

TARGET          = xds-client

PROJECT_NAME    = mock-xds-client
PACKAGE_NAME    = github.com/caitui/mock-xds-client

MAJOR_VERSION   = $(shell cat VERSION)
GIT_VERSION     = $(shell git log -1 --pretty=format:%h)
GIT_NOTES       = $(shell git log -1 --oneline)

# todo 构建环境需要改变
BUILD_IMAGE         = golang:1.19
BUILD_IMAGE_ARM     = golang:1.19

IMAGE_NAME      = xds-client
REPOSITORY      = caitui/${IMAGE_NAME}

TAGS			= ${tags}
TAGS_OPT 		=

# support build custom tags
ifneq ($(TAGS),)
TAGS_OPT 		= -tags ${TAGS}
endif

build:
	docker run --rm -v $(shell pwd):/go/src/${PROJECT_NAME} -w /go/src/${PROJECT_NAME} ${BUILD_IMAGE} make build-local

build-arm:
	docker run --rm -v $(shell pwd):/go/src/${PROJECT_NAME} -w /go/src/${PROJECT_NAME} ${BUILD_IMAGE_ARM} make build-mac

binary: build-arm

build-local:
	@rm -rf build/bundles/${MAJOR_VERSION}/binary
	GO111MODULE=on CGO_ENABLED=1 go build ${TAGS_OPT} \
		-ldflags "-B 0x$(shell head -c20 /dev/urandom|od -An -tx1|tr -d ' \n') -X main.Version=${MAJOR_VERSION}(${GIT_VERSION})" \
		-v -o ${TARGET} \
		${PACKAGE_NAME}/cmd/client/main
	mkdir -p build/bundles/${MAJOR_VERSION}/binary
	mv ${TARGET} build/bundles/${MAJOR_VERSION}/binary
	@cd build/bundles/${MAJOR_VERSION}/binary && $(shell which md5sum) -b ${TARGET} | cut -d' ' -f1  > ${TARGET}.md5

build-mac:
	@rm -rf build/bundles/${MAJOR_VERSION}/binary-arm
	GO111MODULE=on CGO_ENABLED=1 env GOOS=darwin GOARCH=amd64 go build ${TAGS_OPT} \
		-ldflags "-B 0x$(shell head -c20 /dev/urandom|od -An -tx1|tr -d ' \n') -X main.Version=${MAJOR_VERSION}(${GIT_VERSION})" \
		-v -o ${TARGET} \
		${PACKAGE_NAME}/cmd/client/main
	mkdir -p build/bundles/${MAJOR_VERSION}/binary-arm
	mv ${TARGET} build/bundles/${MAJOR_VERSION}/binary-arm
	@cd build/bundles/${MAJOR_VERSION}/binary-arm && $(shell which md5sum) -b ${TARGET} | cut -d' ' -f1  > ${TARGET}.md5

image:
	@rm -rf IMAGEBUILD
	cp -r build/contrib/builder/image IMAGEBUILD && cp build/bundles/${MAJOR_VERSION}/binary/${TARGET} IMAGEBUILD
	docker build --no-cache --rm -t ${IMAGE_NAME}:${MAJOR_VERSION}-${GIT_VERSION} IMAGEBUILD
	docker tag ${IMAGE_NAME}:${MAJOR_VERSION}-${GIT_VERSION} ${REPOSITORY}:${MAJOR_VERSION}-${GIT_VERSION}
	rm -rf IMAGEBUILD

image-arm:
	@rm -rf IMAGEBUILD
	cp -r build/contrib/builder/image IMAGEBUILD && cp build/bundles/${MAJOR_VERSION}/binary-arm/${TARGET} IMAGEBUILD
	docker build --no-cache --rm -t ${IMAGE_NAME}:${MAJOR_VERSION}-${GIT_VERSION} IMAGEBUILD
	docker tag ${IMAGE_NAME}:${MAJOR_VERSION}-${GIT_VERSION} ${REPOSITORY}:${MAJOR_VERSION}-${GIT_VERSION}
	rm -rf IMAGEBUILD

.PHONY: build image upload
