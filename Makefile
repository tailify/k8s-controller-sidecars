IMAGE=ghcr.io/tailify/k8s-controller-sidecars:tailify-3.0.0

build:
	CGO_ENABLED=0 go build -a -installsuffix cgo -o main .

docker:
	docker build -t ${IMAGE} -f Dockerfile .

