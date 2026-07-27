variable "TAG" {
  default = "latest"
}

group "default" {
  targets = ["showdown"]
}

target "showdown" {
  context = "."
  dockerfile = "Dockerfile"
  platforms = ["linux/amd64", "linux/arm64"]
  tags = ["eiladin/movie-night-showdown:${TAG}"]
}
