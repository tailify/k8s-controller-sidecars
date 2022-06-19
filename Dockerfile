FROM golang:1.18-alpine AS build

RUN apk add upx
WORKDIR /go/src/github.com/Riskified/k8s-controller-sidecars
COPY go.mod go.sum /go/src/github.com/Riskified/k8s-controller-sidecars/

RUN go mod download -x

COPY  . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -a -installsuffix cgo -o sidecars-controller .


RUN upx sidecars-controller


FROM alpine:3.16
COPY --from=build /go/src/github.com/Riskified/k8s-controller-sidecars/sidecars-controller /
CMD ["/sidecars-controller"]