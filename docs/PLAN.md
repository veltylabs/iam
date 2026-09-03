# PLAN — lo que falta (lista manual, no despachable)

> Este archivo NO se despacha con `codejob`: todo lo pendiente aquí es
> trabajo humano (cargar credenciales, desplegar, verificar en el
> navegador). Los specs ya ejecutados quedan como historia en
> `docs/PLAN_STAGE_1..6_*.md` y `docs/PLAN_SECURITY_HARDENING.md` — no los
> re-ejecutes, son el registro de lo que produjo el código actual.

## Estado verificado (2026-09-03)

- Panel de administración (etapas 1–6): mergeado a `main` — PR
  [#6](https://github.com/veltylabs/iam/pull/6).
- Endurecimiento de seguridad (I-1…I-8): commiteado en la rama
  `security-hardening`, PR abierto contra `main`. `go vet`, `go test
  ./tests/...` y `GOOS=js GOARCH=wasm go build ./edge/ ./web/` en verde.
  Incluye el bump a las versiones ya endurecidas de `tinywasm/auth`,
  `rbac`, `dom`, `crypto`, `router`, `cloudflare` — ver
  [PLAN_SECURITY_HARDENING.md](PLAN_SECURITY_HARDENING.md) para el detalle
  hallazgo por hallazgo.
- Desarrollo local verificado sin credenciales: `go run ./web` →
  `/api/health` y `/api/health/db` responden `{"ok":true}`, `/` sirve el
  panel (200), `/api/admin/me` sin sesión da 401, y las cabeceras de
  seguridad se emiten. Ver [README](../README.md) §"Desarrollo local".

## Pendiente, en orden

1. **Revisar y mergear el PR del endurecimiento** contra `main`.
2. **Confirmar en Cloudflare, antes o inmediatamente después del deploy que
   dispara el merge**: `IAM_PANEL_ORIGIN` cargada como Secret
   (`https://iam.velty.cl`). **Sin ella el Worker no arranca — ninguna
   ruta, no sólo el panel** (`NewProductionBackend` falla rápido y
   `edge/main.go` nunca llega a `edge.Serve`). Ver
   [DEPLOY.md](DEPLOY.md) §"Variables de ejecución".
3. **Verificación Etapa 7 en un Worker de prueba**: comprobar si
   `GET /api/admin/me` llegaba al Worker antes del cambio de prefijo
   (con `/api/admin/` ya es worker-first por convención; es solo
   confirmación histórica). Requiere Cloudflare.
4. **Revisar el panel en el navegador local** (opcional, visual): levantar
   con `go run ./web`, entrar con la identidad mock (`admin@iam.local`) y
   recorrer proyectos → roles → usuarios → auditoría.
