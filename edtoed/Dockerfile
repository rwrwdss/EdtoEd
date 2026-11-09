FROM golang:1.25.7-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /edtoed-server ./cmd/server

FROM debian:bookworm-slim
WORKDIR /app
COPY --from=build /edtoed-server /usr/local/bin/edtoed-server
COPY models/edtoed_v1.onnx /app/models/edtoed_v1.onnx
EXPOSE 50051 8080
CMD ["edtoed-server"]
