IMG ?= wwyizhang/caksu:latest

# running job clean controller locally 
# make sure to export KUBECONFIG
run: fmt vet build
	./caksu

fmt:
	go fmt ./cmd/...

vet:
	go vet ./cmd/...

install:
	# config RBAC in the default namespace
	kubectl apply -f ./config/

build:
	go build -o caksu ./cmd/...

docker-build:
	@echo "building docker image:" ${IMG}
	docker build -t ${IMG} .
	
docker-push:
	@echo "pushing docker image:" ${IMG} "to Dockerhub"
	docker push ${IMG}
