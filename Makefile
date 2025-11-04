KUBECTL ?= kubectl
K8S_DIR := $(CURDIR)

run-app:
	go run cmd/app/main.go

docker-run-linux:
	./run_with_env.sh

linux-secrets-load:
	export $(cat .env | xargs)

create-secrets:
	$(KUBECTL) create secret generic go-agent-api-secrets --from-env-file=.env
	
create-gcr-secret:
	kubectl create secret docker-registry ghcr-secret \
	  --docker-server=ghcr.io \
	  --docker-username=<USNAME> \
	  --docker-password=<TOKEN> \
	  --docker-email=<EMAIL>

apply:
	$(KUBECTL) apply -f go-agent.yaml

clean:
	$(KUBECTL) delete -f go-agent.yaml --ignore-not-found
