ARG PROXY_CACHE_IMAGE=
ARG GO_IMAGE=docker.io/library/golang
ARG GO_TAG=1.21
ARG COMPONENT
ARG BUILDDIR=/go/src/github.com/meln5674/k8s-smoke-test

FROM ${PROXY_CACHE_IMAGE}${GO_IMAGE}:${GO_TAG} AS build
ARG COMPONENT
ARG BUILDDIR
WORKDIR ${BUILDDIR}
COPY cmd/${COMPONENT}/main.go go.mod go.sum ./
COPY pkg ./pkg
RUN CGO_ENABLED=0 go build -a -tags netgo -ldflags '-w -extldflags "-static"' -o "${COMPONENT}" main.go
RUN ls "${BUILDDIR}/${COMPONENT}"

FROM scratch
ARG BUILDDIR
ARG COMPONENT
COPY --from=build ${BUILDDIR}/${COMPONENT} /entrypoint
ENTRYPOINT ["/entrypoint"]
