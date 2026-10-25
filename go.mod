module github.com/gtsteffaniak/go-media

go 1.27

require (
	github.com/Eyevinn/hi264 v0.10.0
	github.com/Eyevinn/mp4ff v0.50.0
	github.com/abema/go-mp4 v1.7.4
	github.com/at-wat/ebml-go v0.19.4
	github.com/dhowden/tag v0.0.0-20240417053706-3d75831295e8
	github.com/gen2brain/vpx v0.2.1
	golang.org/x/image v0.47.0
)

require github.com/google/uuid v1.1.2 // indirect

replace github.com/Eyevinn/hi264 => ./third_party/hi264
