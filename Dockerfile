FROM --platform=$BUILDPLATFORM golang:alpine AS builder
ARG TARGETOS
ARG TARGETARCH

ENV GO111MODULE=on
WORKDIR /app

COPY . .

RUN go mod download
RUN GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build .

FROM golang:alpine

LABEL com.github.actions.name="LaunchDarkly Find Flags"
LABEL com.github.actions.description="Flags"
LABEL homepage="https://www.launchdarkly.com"

RUN apk update
RUN apk add --no-cache git

COPY --from=builder /app/find-code-references-in-pull-request /usr/bin

ENTRYPOINT ["find-code-references-in-pull-request"]
