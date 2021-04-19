FROM golang:1.15.11-alpine3.13
LABEL com.github.actions.name="LaunchDarkly Find Flags"
LABEL com.github.actions.description="Flags"
LABEL homepage="https://www.launchdarkly.com"

RUN apk update
RUN apk add --no-cache git

RUN mkdir /app
WORKDIR /app
COPY . .
ENV GO111MODULE=on
RUN go mod download
RUN go build .



ENTRYPOINT ["/app/cr-flags"]
