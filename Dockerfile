FROM golang:alpine AS builder

RUN apk add --no-cache git

WORKDIR /app
COPY . .
ENV GO111MODULE=on
RUN go build -mod=vendor -o /find-code-references-in-pull-request .

FROM alpine:3.21

LABEL com.github.actions.name="LaunchDarkly Find Flags"
LABEL com.github.actions.description="Flags"
LABEL homepage="https://www.launchdarkly.com"

RUN apk add --no-cache git

COPY --from=builder /find-code-references-in-pull-request /find-code-references-in-pull-request

ENTRYPOINT ["/find-code-references-in-pull-request"]
