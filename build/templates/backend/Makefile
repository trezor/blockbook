{{define "main" -}}
{{- if ne .Backend.BinaryURL "" }}
ARCHIVE := $(shell basename {{.Backend.BinaryURL}})
{{- else }}
ARCHIVE :=
{{- end }}

all:
	mkdir backend
{{- if ne .Backend.DockerImage "" }}
	docker container inspect extract > /dev/null 2>&1 && docker rm extract || true
	docker create --name extract {{.Backend.DockerImage}}
{{- if eq .Backend.VerificationType "docker"}}
	[ "$$(docker inspect --format='{{`{{index .RepoDigests 0}}`}}' {{.Backend.DockerImage}} | sed 's/.*@sha256://')" = "{{.Backend.VerificationSource}}" ]
{{- end}}
	{{.Backend.ExtractCommand}}
	docker rm extract
{{- else }}
	wget {{.Backend.BinaryURL}}
{{- if eq .Backend.VerificationType "gpg"}}
	wget {{.Backend.VerificationSource}} -O checksum
	gpg --verify checksum ${ARCHIVE}
{{- else if eq .Backend.VerificationType "gpg-sha256"}}
	wget {{.Backend.VerificationSource}} -O checksum
	gpg --verify checksum
	sha256sum -c --ignore-missing checksum
{{- else if eq .Backend.VerificationType "sha256"}}
	[ "$$(sha256sum ${ARCHIVE} | cut -d ' ' -f 1)" = "{{.Backend.VerificationSource}}" ]
{{- end}}
	{{.Backend.ExtractCommand}} ${ARCHIVE}
{{- end}}
{{- if .Backend.ExcludeFiles}}
	# generated from exclude_files
{{- range $index, $name := .Backend.ExcludeFiles}}
	rm backend/{{$name}}
{{- end}}
{{- end}}

clean:
	rm -rf backend
{{- if ne .Backend.BinaryURL "" }}
	rm -f ${ARCHIVE}
{{- end }}
	rm -f checksum
{{end}}
