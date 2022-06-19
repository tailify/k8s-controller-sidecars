IMAGE=284309667046.dkr.ecr.us-east-1.amazonaws.com/riskified/images-library/k8s-controller-sidecars:main-3.0.0

build:
	CGO_ENABLED=0 go build -a -installsuffix cgo -o main .

docker:
	docker build -t ${IMAGE} -f Dockerfile .