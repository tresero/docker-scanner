package security

import (
	"docker-scanner/pkg/models"
	"sort"
)

// CheckImages runs the checkers once on each compose file and adds each issue to
// one image, so it's reported once. An issue goes to the service it names. One
// about the whole file, or about a service with no image row, goes to the file's
// first service by name: compose files are maps, so their order is random. A
// standalone Dockerfile has no compose file to check.
func CheckImages(checkers []Checker, images []models.ImageInfo) {
	rows := map[string]map[string]int{} // file -> service -> index in images
	for i, image := range images {
		if image.Image.Service == "" {
			continue
		}
		if rows[image.File] == nil {
			rows[image.File] = map[string]int{}
		}
		rows[image.File][image.Image.Service] = i
	}

	for file, services := range rows {
		names := make([]string, 0, len(services))
		for name := range services {
			names = append(names, name)
		}
		sort.Strings(names)
		first := services[names[0]]

		for _, issue := range RunAll(checkers, file) {
			i, ok := services[issue.Service]
			if !ok {
				i = first
			}
			images[i].SecurityIssues = append(images[i].SecurityIssues, issue)
		}
	}
}
