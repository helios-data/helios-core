# Builder
FROM golang:1.25.2 AS builder

RUN apt-get update && apt-get install -y --no-install-recommends \
    protobuf-compiler \
    libprotobuf-dev \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download && \
    go install google.golang.org/protobuf/cmd/protoc-gen-go@latest

COPY . ./

RUN mkdir -p generated

RUN find helios-protos -name "*.proto" | xargs protoc \
    --proto_path=helios-protos \
    --go_out=generated \
    --go_opt=paths=source_relative

RUN CGO_ENABLED=0 GOOS=linux go build -o bin/helios ./cmd/helios

# Runtime
FROM alpine:latest

WORKDIR /app

COPY --from=builder /app/bin/ /app/bin

EXPOSE 8080

CMD ["./bin/helios"]