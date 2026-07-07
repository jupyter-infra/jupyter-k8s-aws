# [AWS HYPERPOD]: Configuration for aws-hyperpod deployment mode
{{- if (.Capabilities.APIVersions.Has "helm.toolkit.fluxcd.io/v2beta1") }}
{{- fail "This chart is not compatible with Flux CD. Please use a different deployment method." }}
{{- end }}

{{- if and .Values.clusterWebUI.enabled (not .Values.clusterWebUI.domain) }}
{{- fail "clusterWebUI.domain is required when clusterWebUI.enabled is true" }}
{{- end }}

{{- if .Values.remoteAccess.enabled }}
{{- $mechanisms := include "remoteAccess.mechanisms" . | fromJsonArray }}
{{- if not $mechanisms }}
{{- fail "remoteAccess.enabled is true but no access mechanism is configured. Set remoteAccess.accessMechanisms (or legacy remoteAccess.ssmManagedNodeRole)." }}
{{- end }}
{{- $seen := dict }}
{{- $shellCount := 0 }}
{{- range $mechanisms }}
{{- if not (or (eq .type "ssm") (eq .type "directSSH")) }}
{{- fail (printf "remoteAccess.accessMechanisms: unsupported type %q (must be 'ssm' or 'directSSH')" .type) }}
{{- end }}
{{- if hasKey $seen .type }}
{{- fail (printf "remoteAccess.accessMechanisms: duplicate type %q" .type) }}
{{- end }}
{{- $seen = set $seen .type true }}
{{- $shellCount = add $shellCount 1 }}
{{- $cfg := .configuration | default dict }}
{{- if and (eq .type "ssm") (not $cfg.ssmManagedNodeRole) }}
{{- fail "remoteAccess.accessMechanisms: ssm requires configuration.ssmManagedNodeRole" }}
{{- end }}
{{- if and (eq .type "directSSH") (not $cfg.dnsDomain) }}
{{- fail "remoteAccess.accessMechanisms: directSSH requires configuration.dnsDomain" }}
{{- end }}
{{- end }}
{{- if gt $shellCount 1 }}
{{- fail "remoteAccess.accessMechanisms: ssm and directSSH are mutually exclusive; configure only one" }}
{{- end }}
{{- end }}

{{/* Validate rotator and JWT configuration when clusterWebUI is enabled */}}
{{- if .Values.clusterWebUI.enabled }}

{{/* Parse jwtExpiration to minutes */}}
{{- $jwtExpirationMinutes := 0 }}
{{- if hasSuffix "h" .Values.clusterWebUI.auth.jwtExpiration }}
{{- $jwtExpirationMinutes = (trimSuffix "h" .Values.clusterWebUI.auth.jwtExpiration | int | mul 60) }}
{{- else if hasSuffix "m" .Values.clusterWebUI.auth.jwtExpiration }}
{{- $jwtExpirationMinutes = (trimSuffix "m" .Values.clusterWebUI.auth.jwtExpiration | int) }}
{{- else }}
{{- fail "clusterWebUI.auth.jwtExpiration must end with 'm' (minutes) or 'h' (hours)" }}
{{- end }}

{{/* Parse jwtExpiration to seconds (for sub-minute comparisons) */}}
{{- $jwtExpirationSeconds := (mul $jwtExpirationMinutes 60) }}

{{/* Parse jwtRefreshWindow to seconds */}}
{{- $jwtRefreshWindowSeconds := 0 }}
{{- if hasSuffix "h" .Values.clusterWebUI.auth.jwtRefreshWindow }}
{{- $jwtRefreshWindowSeconds = (trimSuffix "h" .Values.clusterWebUI.auth.jwtRefreshWindow | int | mul 3600) }}
{{- else if hasSuffix "m" .Values.clusterWebUI.auth.jwtRefreshWindow }}
{{- $jwtRefreshWindowSeconds = (trimSuffix "m" .Values.clusterWebUI.auth.jwtRefreshWindow | int | mul 60) }}
{{- else if hasSuffix "s" .Values.clusterWebUI.auth.jwtRefreshWindow }}
{{- $jwtRefreshWindowSeconds = (trimSuffix "s" .Values.clusterWebUI.auth.jwtRefreshWindow | int) }}
{{- else }}
{{- fail "clusterWebUI.auth.jwtRefreshWindow must end with 's', 'm', or 'h'" }}
{{- end }}

{{/* Validate: jwtRefreshWindow <= jwtExpiration */}}
{{- if gt $jwtRefreshWindowSeconds $jwtExpirationSeconds }}
{{- fail (printf "clusterWebUI.auth.jwtRefreshWindow (%s) must be less than or equal to jwtExpiration (%s)" .Values.clusterWebUI.auth.jwtRefreshWindow .Values.clusterWebUI.auth.jwtExpiration) }}
{{- end }}

{{/* Parse jwtRefreshHorizon to seconds */}}
{{- $jwtRefreshHorizonSeconds := 0 }}
{{- if hasSuffix "h" .Values.clusterWebUI.auth.jwtRefreshHorizon }}
{{- $jwtRefreshHorizonSeconds = (trimSuffix "h" .Values.clusterWebUI.auth.jwtRefreshHorizon | int | mul 3600) }}
{{- else if hasSuffix "m" .Values.clusterWebUI.auth.jwtRefreshHorizon }}
{{- $jwtRefreshHorizonSeconds = (trimSuffix "m" .Values.clusterWebUI.auth.jwtRefreshHorizon | int | mul 60) }}
{{- else }}
{{- fail "clusterWebUI.auth.jwtRefreshHorizon must end with 'm' (minutes) or 'h' (hours)" }}
{{- end }}

{{/* Validate: jwtRefreshHorizon >= jwtExpiration */}}
{{- if lt $jwtRefreshHorizonSeconds $jwtExpirationSeconds }}
{{- fail (printf "clusterWebUI.auth.jwtRefreshHorizon (%s) must be greater than or equal to jwtExpiration (%s)" .Values.clusterWebUI.auth.jwtRefreshHorizon .Values.clusterWebUI.auth.jwtExpiration) }}
{{- end }}

{{/* Validate rotator configuration */}}
{{- if .Values.clusterWebUI.rotator.enabled }}
{{- if not .Values.clusterWebUI.rotator.rotationInterval }}
{{- fail "clusterWebUI.rotator.rotationInterval is required when rotator is enabled" }}
{{- end }}
{{- if not (or (hasSuffix "m" .Values.clusterWebUI.rotator.rotationInterval) (hasSuffix "h" .Values.clusterWebUI.rotator.rotationInterval)) }}
{{- fail "clusterWebUI.rotator.rotationInterval must end with 'm' (minutes) or 'h' (hours)" }}
{{- end }}
{{- if not .Values.clusterWebUI.rotator.numberOfKeys }}
{{- fail "clusterWebUI.rotator.numberOfKeys is required when rotator is enabled" }}
{{- end }}
{{- if lt (.Values.clusterWebUI.rotator.numberOfKeys | int) 1 }}
{{- fail "clusterWebUI.rotator.numberOfKeys must be at least 1" }}
{{- end }}

{{/* Parse rotationInterval to minutes */}}
{{- $rotationIntervalMinutes := 0 }}
{{- if hasSuffix "h" .Values.clusterWebUI.rotator.rotationInterval }}
{{- $rotationIntervalMinutes = (trimSuffix "h" .Values.clusterWebUI.rotator.rotationInterval | int | mul 60) }}
{{- else if hasSuffix "m" .Values.clusterWebUI.rotator.rotationInterval }}
{{- $rotationIntervalMinutes = (trimSuffix "m" .Values.clusterWebUI.rotator.rotationInterval | int) }}
{{- end }}

{{/* Validate key retention: numberOfKeys * rotationInterval >= jwtExpiration + 30min buffer */}}
{{- $retentionMinutes := (mul (.Values.clusterWebUI.rotator.numberOfKeys | int) $rotationIntervalMinutes) }}
{{- $requiredRetentionMinutes := (add $jwtExpirationMinutes 30) }}
{{- if lt $retentionMinutes $requiredRetentionMinutes }}
{{- fail (printf "Key retention (%d min) must be >= JWT expiration + 30min buffer (%d min). Increase numberOfKeys or rotationInterval." $retentionMinutes $requiredRetentionMinutes) }}
{{- end }}

{{/* Parse jwtNewKeyUseDelay to seconds */}}
{{- $newKeyUseDelaySeconds := 0 }}
{{- if hasSuffix "h" .Values.clusterWebUI.auth.jwtNewKeyUseDelay }}
{{- $newKeyUseDelaySeconds = (trimSuffix "h" .Values.clusterWebUI.auth.jwtNewKeyUseDelay | int | mul 3600) }}
{{- else if hasSuffix "m" .Values.clusterWebUI.auth.jwtNewKeyUseDelay }}
{{- $newKeyUseDelaySeconds = (trimSuffix "m" .Values.clusterWebUI.auth.jwtNewKeyUseDelay | int | mul 60) }}
{{- else if hasSuffix "s" .Values.clusterWebUI.auth.jwtNewKeyUseDelay }}
{{- $newKeyUseDelaySeconds = (trimSuffix "s" .Values.clusterWebUI.auth.jwtNewKeyUseDelay | int) }}
{{- else }}
{{- fail "clusterWebUI.auth.jwtNewKeyUseDelay must end with 's' (seconds), 'm' (minutes), or 'h' (hours)" }}
{{- end }}

{{/* Validate: jwtNewKeyUseDelay < jwtExpiration */}}
{{- if ge $newKeyUseDelaySeconds $jwtExpirationSeconds }}
{{- fail (printf "clusterWebUI.auth.jwtNewKeyUseDelay (%s = %d sec) must be less than jwtExpiration (%s = %d sec)" .Values.clusterWebUI.auth.jwtNewKeyUseDelay $newKeyUseDelaySeconds .Values.clusterWebUI.auth.jwtExpiration $jwtExpirationSeconds) }}
{{- end }}

{{/* Convert rotationInterval to seconds */}}
{{- $rotationIntervalSeconds := (mul $rotationIntervalMinutes 60) }}

{{/* Validate: jwtNewKeyUseDelay < rotationInterval */}}
{{- if ge $newKeyUseDelaySeconds $rotationIntervalSeconds }}
{{- fail (printf "clusterWebUI.auth.jwtNewKeyUseDelay (%s = %d sec) must be less than rotator.rotationInterval (%s = %d sec)" .Values.clusterWebUI.auth.jwtNewKeyUseDelay $newKeyUseDelaySeconds .Values.clusterWebUI.rotator.rotationInterval $rotationIntervalSeconds) }}
{{- end }}
{{- end }}{{/* end rotator.enabled */}}

{{- end }}{{/* end clusterWebUI.enabled */}}

# This file intentionally does not produce any Kubernetes resources
# It only validates and sets default values