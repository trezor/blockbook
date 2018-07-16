PACKAGEROOT := github.com/schancel/cashaddr-converter

.PHONY: default clean addrconv svc image run
default: addrconv svc

addrconv:
	go build ${PACKAGEROOT}/cmd/addrconv

svc:
	go build ${PACKAGEROOT}/cmd/svc

image:
	docker build -t addrconvsvc .

run:
	docker run -p 8888:3000 addrconvsvc
	
clean:
	rm -vf addrconv svc

