{{/*
Auto-generate secrets if not provided
*/}}
{{- define "defaulter.oauth2ProxyClientSecret" -}}
{{- if .Values.dex.oauth2ProxyClientSecret -}}
{{- .Values.dex.oauth2ProxyClientSecret -}}
{{- else -}}
{{- if not .Values._generated -}}
{{- $_ := set .Values "_generated" dict -}}
{{- end -}}
{{- if not .Values._generated.oauth2ProxyClientSecret -}}
{{- $_ := set .Values._generated "oauth2ProxyClientSecret" (randAlphaNum 32 | lower | trunc 32) -}}
{{- end -}}
{{- .Values._generated.oauth2ProxyClientSecret -}}
{{- end -}}
{{- end -}}

{{/*
Render a map as the comma-separated "k1=v1,k2=v2" list the AWS load balancer annotations
expect. Keys are sorted so the rendered Service is stable across upgrades (Helm map
iteration order is otherwise arbitrary and would churn the annotation on every render).
*/}}
{{- define "traefik.keyValueList" -}}
{{- $map := . -}}
{{- $pairs := list -}}
{{- range $k := keys $map | sortAlpha -}}
{{- $pairs = append $pairs (printf "%s=%s" $k (get $map $k | toString)) -}}
{{- end -}}
{{- join "," $pairs -}}
{{- end -}}

{{/*
Port and scheme each component is reached on, which move together when internalTls
turns on. Defined once so the Service, NetworkPolicy, IngressRoute and ForwardAuth
references can never drift apart.

Emit the port UNQUOTED at the call site: NetworkPolicy and Service ports are
IntOrString, so a quoted "5554" would be read as a named port, not a number.
*/}}
{{- define "defaulter.dexPort" -}}
{{- if and .Values.internalTls.enabled .Values.internalTls.dex -}}5554{{- else -}}5556{{- end -}}
{{- end -}}

{{- define "defaulter.dexScheme" -}}
{{- if and .Values.internalTls.enabled .Values.internalTls.dex -}}https{{- else -}}http{{- end -}}
{{- end -}}

{{- define "defaulter.oauth2ProxyPort" -}}
{{- if and .Values.internalTls.enabled .Values.internalTls.oauth2Proxy -}}4443{{- else -}}4180{{- end -}}
{{- end -}}

{{- define "defaulter.oauth2ProxyScheme" -}}
{{- if and .Values.internalTls.enabled .Values.internalTls.oauth2Proxy -}}https{{- else -}}http{{- end -}}
{{- end -}}

{{/*
Convert rotation interval to cron schedule.
Supports common durations: "Xm" (minutes), "Xh" (hours).
Examples: "5m" converts to every 5 minutes, "1h" converts to every hour.
*/}}
{{- define "defaulter.rotationCronSchedule" -}}
{{- $interval := .Values.rotator.rotationInterval -}}
{{- if hasSuffix "m" $interval -}}
  {{- $minutes := trimSuffix "m" $interval | int -}}
  {{- if eq $minutes 60 -}}
0 * * * *
  {{- else if le $minutes 59 -}}
*/{{ $minutes }} * * * *
  {{- else -}}
  {{- fail (printf "Invalid rotation interval: %s (minutes must be <= 59)" $interval) -}}
  {{- end -}}
{{- else if hasSuffix "h" $interval -}}
  {{- $hours := trimSuffix "h" $interval | int -}}
  {{- if eq $hours 1 -}}
0 * * * *
  {{- else if le $hours 23 -}}
0 */{{ $hours }} * * *
  {{- else -}}
  {{- fail (printf "Invalid rotation interval: %s (hours must be <= 23)" $interval) -}}
  {{- end -}}
{{- else -}}
  {{- fail (printf "Unsupported rotation interval format: %s (use Xm for minutes or Xh for hours)" $interval) -}}
{{- end -}}
{{- end -}}

{{/*
Convert a Go-style duration string (e.g. "8h", "30m", "1h30m") to total seconds.
Supports hours (h) and minutes (m).
*/}}
{{- define "defaulter.durationToSeconds" -}}
{{- $input := . -}}
{{- $totalSeconds := 0 -}}
{{- if regexMatch "^[0-9]+h" $input -}}
  {{- $hours := regexFind "[0-9]+" $input | int -}}
  {{- $totalSeconds = mul $hours 3600 -}}
  {{- $input = regexReplaceAll "^[0-9]+h" $input "" -}}
{{- end -}}
{{- if regexMatch "^[0-9]+m" $input -}}
  {{- $minutes := regexFind "[0-9]+" $input | int -}}
  {{- $totalSeconds = add $totalSeconds (mul $minutes 60) -}}
{{- end -}}
{{- $totalSeconds -}}
{{- end -}}

{{/*
Compute session cookie Max-Age (idle timeout) in seconds from webApp.session.cookieMaxAge.
Sliding: re-Set on every authenticated request. Kept below the session-key retention so the
browser stops sending the cookie before its signing key is pruned (see #86 / validations.tpl).
*/}}
{{- define "defaulter.sessionCookieMaxAgeSecs" -}}
{{- include "defaulter.durationToSeconds" .Values.webApp.session.cookieMaxAge -}}
{{- end -}}

{{/*
Compute session max lifetime in seconds.
Equal to the OAuth2 Proxy cookie expiry (absolute upper bound).
*/}}
{{- define "defaulter.sessionMaxLifetimeSecs" -}}
{{- include "defaulter.durationToSeconds" .Values.oauth2Proxy.cookieExpire -}}
{{- end -}}

{{/*
Compute session near-expiry threshold in seconds.
Stop refreshing the session cookie when the OAuth2 token is within this window of expiry.
Set to 25% of cookie expiry (i.e. the remaining 25% after cookieMaxAge).
*/}}
{{- define "defaulter.sessionNearExpiryThresholdSecs" -}}
{{- $expirySecs := include "defaulter.durationToSeconds" .Values.oauth2Proxy.cookieExpire | int -}}
{{- div $expirySecs 4 -}}
{{- end -}}
