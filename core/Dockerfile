FROM golang:1.25 AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
	go build -trimpath -ldflags="-s -w" -o /out/aitask-backend ./cmd/server
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
	go build -trimpath -ldflags="-s -w" -o /out/healthcheck ./cmd/healthcheck
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
	go build -trimpath -ldflags="-s -w" -o /out/aitask-openviking ./cmd/openviking
RUN mkdir -p /out/openviking-data

FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app

COPY --from=builder /out/aitask-backend /app/aitask-backend
COPY --from=builder /out/healthcheck /app/healthcheck
COPY --from=builder /out/aitask-openviking /app/aitask-openviking
COPY --from=builder --chown=nonroot:nonroot /out/openviking-data /data

EXPOSE 8080
EXPOSE 9090

USER nonroot:nonroot

ENTRYPOINT ["/app/aitask-backend"]
