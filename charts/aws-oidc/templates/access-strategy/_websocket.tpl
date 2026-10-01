{{/*
Remote IDE access over SSH-in-WebSocket, shared by the OAuth and bearer strategies.

WebSocket is a transport bolted onto an existing authentication mode, not an
authentication mode of its own — the same shape as remoteAccess (SSM) and
directSSH in the aws-hyperpod chart. Keeping it a modifier means a workspace
already referencing a strategy gains remote access when the flag is turned on,
rather than needing a new strategy and therefore a new workspace.
*/}}

{{/*
The /ssh-ws IngressRoute. Deliberately a dedicated sub-path rather than a match on
the Upgrade header, which would also capture JupyterLab's own kernel and terminal
WebSockets. No compress middleware: the payload is an SSH stream, already opaque.
*/}}
{{- define "awsOidc.webSocket.ingressRoute" -}}
- kind: IngressRoute
  apiVersion: traefik.io/v1alpha1
  namePrefix: websocket-route
  template: |
    spec:
      entryPoints:
        - websecure
      routes:
        - match: "Host(`{{ .Values.domain }}`) && PathPrefix(`/workspaces/{{`{{ .Workspace.Namespace }}`}}/{{`{{ .Workspace.Name }}`}}/ssh-ws`)"
          kind: Rule
          priority: 120
          middlewares:
            - name: auth-headers
              namespace: {{ .Values.namespace }}
            - name: authmiddleware-bearer-auth
              namespace: {{ .Values.namespace }}
          services:
            - name: "{{`{{ .Service.Name }}`}}"
              namespace: "{{`{{ .Service.Namespace }}`}}"
              port: {{ .Values.accessStrategy.webSocket.port }}
{{- end }}

{{/*
The proxy sidecar and the Service port it is published on. The SSH server itself
runs in the workspace container, so SSH_HOST_KEY_PATH is merged there and not here.
*/}}
{{- define "awsOidc.webSocket.podModifications" -}}
additionalContainers:
  - name: {{ .Values.accessStrategy.webSocket.containerName }}
    image: "{{ .Values.accessStrategy.webSocket.image.repository }}/{{ .Values.accessStrategy.webSocket.image.imageName }}:{{ .Values.accessStrategy.webSocket.image.imageTag }}"
    imagePullPolicy: {{ .Values.accessStrategy.webSocket.image.imagePullPolicy }}
    securityContext:
      allowPrivilegeEscalation: false
      readOnlyRootFilesystem: true
      runAsNonRoot: true
      runAsUser: 65532
      capabilities:
        drop:
        - ALL
    ports:
      - name: {{ .Values.accessStrategy.webSocket.portName }}
        containerPort: {{ .Values.accessStrategy.webSocket.port }}
        protocol: TCP
    env:
      - name: LISTEN_ADDR
        value: ":{{ .Values.accessStrategy.webSocket.port }}"
      - name: TARGET_HOST
        value: "{{ .Values.accessStrategy.webSocket.targetHost }}"
      - name: TARGET_PORT
        value: "{{ .Values.accessStrategy.webSocket.targetPort }}"
      - name: MAX_SESSION_DURATION
        value: "{{ .Values.accessStrategy.webSocket.maxSessionDuration }}"
      - name: MAX_CONNECTIONS
        value: "{{ .Values.accessStrategy.webSocket.maxConnections }}"
      - name: READ_LIMIT
        value: "{{ .Values.accessStrategy.webSocket.readLimit }}"
      - name: PING_INTERVAL
        value: "{{ .Values.accessStrategy.webSocket.pingInterval }}"
      - name: PING_TIMEOUT
        value: "{{ .Values.accessStrategy.webSocket.pingTimeout }}"
      - name: TARGET_HEALTH_INTERVAL
        value: "{{ .Values.accessStrategy.webSocket.targetHealthInterval }}"
    # /health reports on the proxy process alone and never dials the target, so a
    # broken tunnel cannot withdraw the workspace's other ports. Target
    # reachability is published as ws_proxy_target_reachable instead.
    livenessProbe:
      httpGet:
        path: /health
        port: {{ .Values.accessStrategy.webSocket.port }}
      periodSeconds: 10
      failureThreshold: 3
    readinessProbe:
      httpGet:
        path: /health
        port: {{ .Values.accessStrategy.webSocket.port }}
      initialDelaySeconds: 2
      periodSeconds: 5
    resources:
      {{- toYaml .Values.accessStrategy.webSocket.resources | nindent 6 }}
exposedPorts:
  - {{ .Values.accessStrategy.webSocket.portName }}
{{- end }}

{{/*
Ingress on the proxy port, appended to a per-workspace NetworkPolicy peer that
already allows the application port.
*/}}
{{- define "awsOidc.webSocket.netpolPort" -}}
- port: {{ .Values.accessStrategy.webSocket.port }}
  protocol: TCP
{{- end }}
