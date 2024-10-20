IMAGE_TAG = "maxit-worker"
run:
	docker build -t $(IMAGE_TAG) . && docker run -it --rm $(IMAGE_TAG)