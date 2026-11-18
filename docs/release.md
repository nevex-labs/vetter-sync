# Release del agente

## Una vez: clave de firma

La clave privada vive solo en GitHub Secrets; la pública se compila en el binario.

```bash
openssl genpkey -algorithm ED25519 -out signing.pem
gh secret set UPDATE_SIGNING_KEY < signing.pem
PUB=$(openssl pkey -in signing.pem -pubout -outform DER | tail -c 32 | base64 -w0)
gh variable set UPDATE_PUBLIC_KEY --body "$PUB"
rm signing.pem
```

Guardá una copia de la privada en el gestor de secretos de Nevex. Si se pierde,
hay que generar otra y publicar un binario con la pública nueva: los agentes
viejos no la conocen y necesitan una actualización manual una vez.

## Cada release

```bash
git tag v0.2.0
git push origin v0.2.0
```

El workflow `.github/workflows/release.yml` hace: tests, `go vet`, build de
Windows con la versión del tag, `sha256`, manifiesto, firma, verificación y
`gh release create` con `vetter-agent.exe`, `manifest.json` y `manifest.sig`.

## Verificar

```bash
gh release view v0.2.0
curl -sL https://github.com/nevex-labs/vetter-sync/releases/latest/download/manifest.json
```

El agente lo toma en el próximo chequeo (máximo 24 h). No hay que tocar la PC
ni el panel.

## Rollback

No se baja un release: el agente solo avanza. Si una versión sale mal, se
publica otra con un número mayor y el código anterior. Además, el agente
restaura `.old` solo si la nueva falla 3 corridas seguidas.
