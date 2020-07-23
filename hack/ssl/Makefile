APP = oam-kubernetes-runtime-webhook
NAMESPACE = oam-system

.PHONY:cert
cert:
	@./ssl.sh $(APP) $(NAMESPACE)

.PHONY:clean
clean:
	rm -vf *.key *.pem *.cert *.crt *.csr
