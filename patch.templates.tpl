{{ range $i, $m := (reverse .Manifests) }}
{{ if ne $m.kind "Namespace" }}
{{ $i := sub (len $.Manifests) (add $i 1) }}
- op: delete
  path: {{ $i }}
{{ end }}
{{ end }}

{{ $namespace := .Namespace }}
{{ range $name, $spec := .Workloads }}
{{ $service := $spec.service }}
{{ $resources := $spec.resources }}
{{ $firstContainerName := index (keys $spec.containers) 0 }}
{{ $firstContainer := get $spec.containers $firstContainerName }}
- op: set
  path: -1
  value:
    apiVersion: kro.run/v1alpha1
    kind: Workload
    metadata:
      name: {{ $name }}
      {{ if ne $namespace "" }}
      namespace: {{ $namespace }}
      {{ end }}
    spec:
      image: {{ $firstContainer.image }}
      {{- if and $firstContainer.variables (gt (len $firstContainer.variables) 0) }}
      env:
        {{- range $variableName, $variableValue := $firstContainer.variables }}
        - name: {{ $variableName }}
          value: {{ substituteValue $name $variableValue }}
        {{ end }}
      {{ end }}
      {{- range $resourceName, $resource := $resources }}
      {{- if eq $resource.type "route" }}
      route:
        host: {{ substituteValue $name $resource.params.host }}
        path: {{ $resource.params.path }}
        port: {{ $resource.params.port }}
      {{ end }}
      {{ end }}
{{ end }}