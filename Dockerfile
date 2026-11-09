# build stage
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG REVISION=unknown
RUN CGO_ENABLED=0 go build -ldflags "-s -w -X main.revision=${REVISION}" -trimpath -o /dummypage ./app

# run stage — minimal scratch image, just the static binary
FROM scratch
COPY --from=build /dummypage /dummypage
EXPOSE 8080
ENV GOMEMLIMIT=16MiB LISTEN_HOST=0.0.0.0 LISTEN_PORT=8080
ENTRYPOINT ["/dummypage"]
