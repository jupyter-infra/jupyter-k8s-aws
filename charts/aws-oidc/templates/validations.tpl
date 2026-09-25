# [AWS OIDC]: Configuration for aws-oidc deployment mode
{{- if (.Capabilities.APIVersions.Has "helm.toolkit.fluxcd.io/v2beta1") }}
{{- fail "This chart is not compatible with Flux CD. Please use a different deployment method." }}
{{- end }}

{{- if not .Values.domain }}
{{- fail "domain is required" }}
{{- end }}

{{- if .Values.storageClass.efs.parameters.fileSystemId }}
{{- if not .Values.storageClass.efs.parameters.fileSystemId }}
{{- fail ".storageClass.efs.parameters.fileSystemId is required" }}
{{- end }}
{{- end }}

{{/* The whole edge depends on this one value, and a wrong one fails only as a Service event
     minutes later when the provider cannot create the TLS listener. Shape-check it here.
     The partition is matched loosely (aws, aws-cn, aws-us-gov, aws-iso*) so the guard does not
     have to be revised for a new partition. */}}
{{- if not .Values.tls.acm.certificateArn }}
{{- fail "tls.acm.certificateArn is required" }}
{{- end }}
{{- if not (regexMatch "^arn:aws[a-z-]*:acm:[a-z0-9-]+:[0-9]{12}:certificate/[a-zA-Z0-9-]+$" .Values.tls.acm.certificateArn) }}
{{- fail (printf "tls.acm.certificateArn must be an ACM certificate ARN (arn:<partition>:acm:<region>:<account>:certificate/<id>), got %q" .Values.tls.acm.certificateArn) }}
{{- end }}

{{/* The default pins a TLS 1.2 minimum. An empty value still renders the annotation, and the
     provider then passes SslPolicy: "" through to a listener creation that errors out. */}}
{{- if not .Values.tls.acm.sslPolicy }}
{{- fail "tls.acm.sslPolicy must not be empty — it sets the NLB listener's TLS security policy" }}
{{- end }}

{{/* Guard the removed Let's Encrypt keys explicitly. With --reset-then-reuse-values a stored
     value from an older release would otherwise be silently ignored, leaving the operator to
     think TLS is still configured the old way. */}}
{{- if .Values.certManager }}
{{- fail "certManager.* was removed — TLS now terminates at the NLB with tls.acm.certificateArn (Let's Encrypt is no longer supported)" }}
{{- end }}
{{- if .Values.tls.mode }}
{{- fail "tls.mode was removed — ACM is the only supported mode; drop the key and set tls.acm.certificateArn" }}
{{- end }}

{{/* Validate: reject the two components whose images cannot serve TLS yet. Failing here beats
     rendering an https:// upstream that every request would then fail to reach. */}}
{{- if and .Values.internalTls.enabled .Values.internalTls.authmiddleware }}
{{- fail "internalTls.authmiddleware is not supported yet — the authmiddleware image cannot serve TLS (see jupyter-infra/jupyter-k8s#479)" }}
{{- end }}
{{- if and .Values.internalTls.enabled .Values.internalTls.webApp }}
{{- fail "internalTls.webApp is not supported yet — the jupyter-k8s-ui image cannot serve TLS (see jupyter-infra/jupyter-k8s-ui#71)" }}
{{- end }}

{{/* Validate: the nodes the NLB targets must be able to run Traefik. targetNodeLabels selects
     which nodes join the target group; the effective node selector decides where Traefik pods
     land. Labelling the NLB for one tier while pinning Traefik to another yields a target group
     that can never pass a health check — a total edge outage with no render and no apply error.
     Only checked when both are set: an empty targetNodeLabels registers every node, and an empty
     selector lets Traefik run anywhere. */}}
{{- if .Values.traefik.targetNodeLabels }}
{{- $selector := .Values.traefik.nodeSelector | default .Values.nodeSelector }}
{{- if $selector }}
{{- range $k, $v := .Values.traefik.targetNodeLabels }}
{{- if not (hasKey $selector $k) }}
{{- fail (printf "traefik.targetNodeLabels has %q, which the effective Traefik node selector does not constrain — the NLB would target nodes that cannot run Traefik" $k) }}
{{- else if ne (get $selector $k | toString) ($v | toString) }}
{{- fail (printf "traefik.targetNodeLabels %s=%v conflicts with the Traefik node selector %s=%v — the NLB would target nodes that cannot run Traefik" $k $v $k (get $selector $k)) }}
{{- end }}
{{- end }}
{{- end }}
{{- end }}

{{- if not .Values.github.clientId }}
{{- fail "github.clientId is required" }}
{{- end }}

{{- if not .Values.github.clientSecret }}
{{- fail "github.clientSecret is required" }}
{{- end }}

{{- if not .Values.github.orgs }}
{{- fail "At least one organization must be specified" }}
{{- end }}

{{- if not .Values.oauth2Proxy.cookieSecret }}
{{- fail "oauth2Proxy.cookieSecret is required" }}
{{- end }}

{{/* Validate: bearer access strategy requires enableBearerAuth */}}
{{- if and .Values.accessStrategy.createBearer (not .Values.authmiddleware.enableBearerAuth) }}
{{- fail "accessStrategy.createBearer requires authmiddleware.enableBearerAuth to be true" }}
{{- end }}

{{/* Validate: the /ssh-ws route is guarded by the bearer-auth ForwardAuth handler */}}
{{- if and .Values.accessStrategy.webSocket.enabled (not .Values.authmiddleware.enableBearerAuth) }}
{{- fail "accessStrategy.webSocket.enabled requires authmiddleware.enableBearerAuth to be true" }}
{{- end }}

{{/* Validate: WebSocket is a transport on a strategy, so at least one must exist */}}
{{- if and .Values.accessStrategy.webSocket.enabled (not (or .Values.accessStrategy.createOAuth .Values.accessStrategy.createBearer)) }}
{{- fail "accessStrategy.webSocket.enabled requires accessStrategy.createOAuth or accessStrategy.createBearer" }}
{{- end }}

{{/* Validate: additionalNamespaces must not include the default workspace namespace (it is added automatically) */}}
{{- if has .Values.webApp.workspacesDefaultNamespace .Values.webApp.workspaceNamespaceSelection.additionalNamespaces }}
{{- fail (printf "webApp.workspaceNamespaceSelection.additionalNamespaces must not contain the default workspace namespace %q — it is always included automatically" .Values.webApp.workspacesDefaultNamespace) }}
{{- end }}

{{/* Validate: additionalNamespaces must not contain duplicates (| default list tolerates a null override) */}}
{{- $additional := .Values.webApp.workspaceNamespaceSelection.additionalNamespaces | default list }}
{{- if ne (len $additional) (len (uniq $additional)) }}
{{- fail (printf "webApp.workspaceNamespaceSelection.additionalNamespaces must not contain duplicates, got: %v" $additional) }}
{{- end }}

{{/* Validate: the pre-rename webApp.namespace key is no longer read (renamed to workspacesDefaultNamespace).
     Guard explicitly — with --reset-then-reuse-values a stored old value would otherwise be silently dropped. */}}
{{- if .Values.webApp.namespace }}
{{- fail "webApp.namespace was renamed to webApp.workspacesDefaultNamespace — move the value to the new key" }}
{{- end }}

{{/* Validate JWT refresh settings */}}
{{- $jwtRefreshWindowSeconds := 0 }}
{{- if hasSuffix "h" .Values.authmiddleware.jwtRefreshWindow }}
{{- $jwtRefreshWindowSeconds = (trimSuffix "h" .Values.authmiddleware.jwtRefreshWindow | int | mul 3600) }}
{{- else if hasSuffix "m" .Values.authmiddleware.jwtRefreshWindow }}
{{- $jwtRefreshWindowSeconds = (trimSuffix "m" .Values.authmiddleware.jwtRefreshWindow | int | mul 60) }}
{{- else if hasSuffix "s" .Values.authmiddleware.jwtRefreshWindow }}
{{- $jwtRefreshWindowSeconds = (trimSuffix "s" .Values.authmiddleware.jwtRefreshWindow | int) }}
{{- else }}
{{- fail "authmiddleware.jwtRefreshWindow must end with 's', 'm', or 'h'" }}
{{- end }}

{{- $jwtExpirationSecondsForRefresh := 0 }}
{{- if hasSuffix "h" .Values.authmiddleware.jwtExpiration }}
{{- $jwtExpirationSecondsForRefresh = (trimSuffix "h" .Values.authmiddleware.jwtExpiration | int | mul 3600) }}
{{- else if hasSuffix "m" .Values.authmiddleware.jwtExpiration }}
{{- $jwtExpirationSecondsForRefresh = (trimSuffix "m" .Values.authmiddleware.jwtExpiration | int | mul 60) }}
{{- end }}

{{/* Validate: jwtRefreshWindow <= jwtExpiration */}}
{{- if gt $jwtRefreshWindowSeconds $jwtExpirationSecondsForRefresh }}
{{- fail (printf "authmiddleware.jwtRefreshWindow (%s) must be less than or equal to jwtExpiration (%s)" .Values.authmiddleware.jwtRefreshWindow .Values.authmiddleware.jwtExpiration) }}
{{- end }}

{{- $jwtRefreshHorizonSeconds := 0 }}
{{- if hasSuffix "h" .Values.authmiddleware.jwtRefreshHorizon }}
{{- $jwtRefreshHorizonSeconds = (trimSuffix "h" .Values.authmiddleware.jwtRefreshHorizon | int | mul 3600) }}
{{- else if hasSuffix "m" .Values.authmiddleware.jwtRefreshHorizon }}
{{- $jwtRefreshHorizonSeconds = (trimSuffix "m" .Values.authmiddleware.jwtRefreshHorizon | int | mul 60) }}
{{- else }}
{{- fail "authmiddleware.jwtRefreshHorizon must end with 'm' (minutes) or 'h' (hours)" }}
{{- end }}

{{/* Validate: jwtRefreshHorizon >= jwtExpiration */}}
{{- if lt $jwtRefreshHorizonSeconds $jwtExpirationSecondsForRefresh }}
{{- fail (printf "authmiddleware.jwtRefreshHorizon (%s) must be greater than or equal to jwtExpiration (%s)" .Values.authmiddleware.jwtRefreshHorizon .Values.authmiddleware.jwtExpiration) }}
{{- end }}

{{/* Validate: dex startupProbe keyCheck budget yields a valid failureThreshold (see #82).
     deployment.yaml computes failureThreshold = timeout_seconds / period_seconds (integer
     div). A period of 0 divides by zero, and period > timeout truncates to 0 — either way
     Kubernetes rejects a startupProbe with failureThreshold: 0, so guard both here. */}}
{{- if lt (.Values.dex.keyCheck.period_seconds | int) 1 }}
{{- fail (printf "dex.keyCheck.period_seconds must be at least 1, got %d" (.Values.dex.keyCheck.period_seconds | int)) }}
{{- end }}
{{- if lt (.Values.dex.keyCheck.timeout_seconds | int) (.Values.dex.keyCheck.period_seconds | int) }}
{{- fail (printf "dex.keyCheck.timeout_seconds (%d) must be >= period_seconds (%d) so the startupProbe failureThreshold is at least 1" (.Values.dex.keyCheck.timeout_seconds | int) (.Values.dex.keyCheck.period_seconds | int)) }}
{{- end }}

{{/* Validate: web-app session-signing key retention must cover the cookie's idle Max-Age (#86).
     The fast-path cookie (workspace_console_session) is re-signed with the NEWEST key on every
     request and sent by the browser until its (sliding) Max-Age. If its key is pruned first, the
     web-app can't decode it and — because the fast-path IngressRoute bypasses OAuth2 Proxy — the
     request dead-ends tokenless. Mirroring the authmiddleware retention guard, retention must
     exceed cookieMaxAge by at least one rotationInterval (cookies always carry the newest key).
     Retention = webApp.session.numberOfKeys * rotator.rotationInterval. */}}
{{- if and .Values.webApp.enabled .Values.rotator.enabled }}
{{- $cookieMaxAgeSecs := include "defaulter.sessionCookieMaxAgeSecs" . | int }}
{{- $maxLifetimeSecs := include "defaulter.sessionMaxLifetimeSecs" . | int }}
{{- if lt $cookieMaxAgeSecs 1 }}
{{- fail (printf "webApp.session.cookieMaxAge must be a positive duration (e.g. \"1h\"), got %q" .Values.webApp.session.cookieMaxAge) }}
{{- end }}
{{- if ge $cookieMaxAgeSecs $maxLifetimeSecs }}
{{- fail (printf "webApp.session.cookieMaxAge (%s) must be less than the session max lifetime (%ds, = oauth2Proxy.cookieExpire %s)" .Values.webApp.session.cookieMaxAge $maxLifetimeSecs .Values.oauth2Proxy.cookieExpire) }}
{{- end }}
{{- $rotationSecs := include "defaulter.durationToSeconds" .Values.rotator.rotationInterval | int }}
{{- $retentionSecs := mul (.Values.webApp.session.numberOfKeys | int) $rotationSecs }}
{{- $requiredSecs := add $cookieMaxAgeSecs $rotationSecs }}
{{- if lt $retentionSecs $requiredSecs }}
{{- fail (printf "web-app session key retention (%ds = numberOfKeys %d * rotationInterval %s) must be >= cookieMaxAge (%s) + one rotationInterval (%ds). Increase webApp.session.numberOfKeys or rotator.rotationInterval, or lower webApp.session.cookieMaxAge." $retentionSecs (.Values.webApp.session.numberOfKeys | int) .Values.rotator.rotationInterval .Values.webApp.session.cookieMaxAge $requiredSecs) }}
{{- end }}
{{- end }}

{{/* Validate rotator configuration if enabled */}}
{{- if .Values.rotator.enabled }}
{{- if not .Values.rotator.rotationInterval }}
{{- fail "rotator.rotationInterval is required when rotator is enabled" }}
{{- end }}
{{- if not (or (hasSuffix "m" .Values.rotator.rotationInterval) (hasSuffix "h" .Values.rotator.rotationInterval)) }}
{{- fail "rotator.rotationInterval must end with 'm' (minutes) or 'h' (hours)" }}
{{- end }}
{{- if not .Values.rotator.numberOfKeys }}
{{- fail "rotator.numberOfKeys is required when rotator is enabled" }}
{{- end }}
{{- if lt (.Values.rotator.numberOfKeys | int) 1 }}
{{- fail "rotator.numberOfKeys must be at least 1" }}
{{- end }}

{{/* Validate key retention is sufficient for JWT expiration */}}
{{- $jwtExpirationMinutes := 0 }}
{{- if hasSuffix "h" .Values.authmiddleware.jwtExpiration }}
{{- $jwtExpirationMinutes = (trimSuffix "h" .Values.authmiddleware.jwtExpiration | int | mul 60) }}
{{- else if hasSuffix "m" .Values.authmiddleware.jwtExpiration }}
{{- $jwtExpirationMinutes = (trimSuffix "m" .Values.authmiddleware.jwtExpiration | int) }}
{{- else }}
{{- fail "authmiddleware.jwtExpiration must end with 'm' (minutes) or 'h' (hours)" }}
{{- end }}

{{- $rotationIntervalMinutes := 0 }}
{{- if hasSuffix "h" .Values.rotator.rotationInterval }}
{{- $rotationIntervalMinutes = (trimSuffix "h" .Values.rotator.rotationInterval | int | mul 60) }}
{{- else if hasSuffix "m" .Values.rotator.rotationInterval }}
{{- $rotationIntervalMinutes = (trimSuffix "m" .Values.rotator.rotationInterval | int) }}
{{- end }}

{{- $retentionMinutes := (mul (.Values.rotator.numberOfKeys | int) $rotationIntervalMinutes) }}
{{- $requiredRetentionMinutes := (add $jwtExpirationMinutes 30) }}
{{- if lt $retentionMinutes $requiredRetentionMinutes }}
{{- fail (printf "Key retention (%d min) must be >= JWT expiration + 30min buffer (%d min). Increase numberOfKeys or rotationInterval." $retentionMinutes $requiredRetentionMinutes) }}
{{- end }}

{{/* Validate jwtNewKeyUseDelay */}}
{{- $newKeyUseDelaySeconds := 0 }}
{{- if hasSuffix "h" .Values.authmiddleware.jwtNewKeyUseDelay }}
{{- $newKeyUseDelaySeconds = (trimSuffix "h" .Values.authmiddleware.jwtNewKeyUseDelay | int | mul 3600) }}
{{- else if hasSuffix "m" .Values.authmiddleware.jwtNewKeyUseDelay }}
{{- $newKeyUseDelaySeconds = (trimSuffix "m" .Values.authmiddleware.jwtNewKeyUseDelay | int | mul 60) }}
{{- else if hasSuffix "s" .Values.authmiddleware.jwtNewKeyUseDelay }}
{{- $newKeyUseDelaySeconds = (trimSuffix "s" .Values.authmiddleware.jwtNewKeyUseDelay | int) }}
{{- else }}
{{- fail "authmiddleware.jwtNewKeyUseDelay must end with 's' (seconds), 'm' (minutes), or 'h' (hours)" }}
{{- end }}

{{/* Convert jwtExpiration to seconds */}}
{{- $jwtExpirationSeconds := 0 }}
{{- if hasSuffix "h" .Values.authmiddleware.jwtExpiration }}
{{- $jwtExpirationSeconds = (trimSuffix "h" .Values.authmiddleware.jwtExpiration | int | mul 3600) }}
{{- else if hasSuffix "m" .Values.authmiddleware.jwtExpiration }}
{{- $jwtExpirationSeconds = (trimSuffix "m" .Values.authmiddleware.jwtExpiration | int | mul 60) }}
{{- else if hasSuffix "s" .Values.authmiddleware.jwtExpiration }}
{{- $jwtExpirationSeconds = (trimSuffix "s" .Values.authmiddleware.jwtExpiration | int) }}
{{- end }}

{{/* Convert rotationInterval to seconds */}}
{{- $rotationIntervalSeconds := (mul $rotationIntervalMinutes 60) }}

{{/* Validate: jwtNewKeyUseDelay < jwtExpiration */}}
{{- if ge $newKeyUseDelaySeconds $jwtExpirationSeconds }}
{{- fail (printf "authmiddleware.jwtNewKeyUseDelay (%s = %d sec) must be less than jwtExpiration (%s = %d sec)" .Values.authmiddleware.jwtNewKeyUseDelay $newKeyUseDelaySeconds .Values.authmiddleware.jwtExpiration $jwtExpirationSeconds) }}
{{- end }}

{{/* Validate: jwtNewKeyUseDelay < rotationInterval */}}
{{- if ge $newKeyUseDelaySeconds $rotationIntervalSeconds }}
{{- fail (printf "authmiddleware.jwtNewKeyUseDelay (%s = %d sec) must be less than rotator.rotationInterval (%s = %d sec)" .Values.authmiddleware.jwtNewKeyUseDelay $newKeyUseDelaySeconds .Values.rotator.rotationInterval $rotationIntervalSeconds) }}
{{- end }}
{{- end }}

# This file intentionally does not produce any Kubernetes resources
# It only validates and sets default values