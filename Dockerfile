FROM golang:1.19 AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -mod=readonly -v -o /bin/serverless_autoneg_controller ./cmd/operator

FROM gcr.io/distroless/static
COPY --from=builder /bin/serverless_autoneg_controller /bin/serverless_autoneg_controller
ENTRYPOINT [ "/bin/serverless_autoneg_controller" ]
