# Despliegue de `iam` — el camino único

Mismo mecanismo que [`veltylabs/misitio`](https://github.com/veltylabs/misitio/blob/main/docs/DEPLOY.md):
GitHub Actions corre `goflare build` + `goflare deploy` (Go puro, sin Node ni
Wrangler — ver `AGENTS.md`). `iam` es un Worker con panel (assets estáticos +
`client.wasm` en `web/public`).

## Las dos clases de variable

| | Dónde vive | Quién la lee | Cuándo |
|---|---|---|---|
| **De despliegue** | GitHub → Secrets y `env:` del workflow | la Action, para hablarle a Cloudflare | al hacer push |
| **De ejecución** | Cloudflare → Worker `iam` (como Secret) | el Worker, con `env.Reader` (`cloudflare.Env` en producción) | en cada petición |

### De despliegue — GitHub Secrets (3) y `deploy.yml`

`.github/workflows/deploy.yml` configura el paso `Deploy with goflare` con:

| Clave | Dónde se define | Para qué |
|---|---|---|
| `CLOUDFLARE_ACCOUNT_ID` | Secret | Identifica la cuenta `velty` |
| `CLOUDFLARE_API_TOKEN` | Secret | Permisos de **Workers Scripts: Edit** y **Workers Routes: Edit** |
| `D1_DATABASE_ID` | Secret | ID de `velty-iam-db` |
| `PROJECT_NAME`/`WORKER_NAME` | inline (`iam`) | Nombre del Worker |
| `DOMAIN` | inline (`iam.velty.cl`) | Dominio personalizado |
| `D1_DATABASE_NAME` | inline (`DB`) | Nombre del binding exigido por `routes.BindingD1` |

Van por repositorio (GitHub Free no comparte secretos de organización con
repos privados):

```bash
gh secret set CLOUDFLARE_ACCOUNT_ID --repo veltylabs/iam
gh secret set CLOUDFLARE_API_TOKEN  --repo veltylabs/iam
gh secret set D1_DATABASE_ID        --repo veltylabs/iam
```

### De ejecución — variables del Worker en Cloudflare (6)

El Worker las lee con `env.Reader` (`cloudflare.Env` en `edge/main.go`, ver
`config/auth.go`). **Sin las tres de Google, sin `JWT_SECRET` y sin
`IAM_PANEL_ORIGIN` no arranca** —
deliberado: un servicio de identidad que arranca a medias es peor que uno que
no arranca. `IAM_ADMIN_EMAILS` no impide el arranque, pero sin ella nadie
puede entrar al panel.

Cloudflare → Workers & Pages → `iam` → *Settings* → *Variables and Secrets*,
las seis como tipo **Secret** (no texto plano — un `PUT` de script que no
declara un binding `secret_text` en su metadata lo borra si era texto plano,
pero lo conserva si era Secret):

| Variable | Obligatoria | Qué pasa si falta |
|---|---|---|
| `GOOGLE_CLIENT_ID` | sí | el Worker no arranca |
| `GOOGLE_CLIENT_SECRET` | sí | el Worker no arranca |
| `GOOGLE_REDIRECT_URL` | sí | `https://iam.velty.cl/oauth/callback/google`, registrada en Google Cloud Console |
| `JWT_SECRET` | sí | el Worker no arranca — firma la cookie SSO y los tokens de autorización |
| `IAM_PANEL_ORIGIN` | sí | el Worker no arranca — origen exacto del panel (`https://iam.velty.cl`); las mutaciones de `/api/admin/*` solo aceptan peticiones de este origen (ver `docs/ARCHITECTURE.md` §8.7) |
| `IAM_ADMIN_EMAILS` | sí | el panel de administración no admite a nadie (lista de correos separada por coma; ver `docs/ARCHITECTURE.md` §8.2) |

## El camino, en orden

### 0 · D1 y el Account ID *(una vez)*

`velty-iam-db` ya existe en la cuenta `velty` (creada 2026-08-25). Copia su
`uuid` para el secreto `D1_DATABASE_ID`, y el Account ID de la cuenta para
`CLOUDFLARE_ACCOUNT_ID`.

### 1 · Crear el token y los tres secretos de GitHub *(una vez)*

El token: Cloudflare → *My Profile* → *API Tokens* → *Create Token*, con
`Account · Workers Scripts · Edit` y `Zone · Workers Routes · Edit` (sobre la
zona `velty.cl`).

### 2 · Primer push — despliegue inicial

Cualquier push a `main` dispara la Action. `goflare deploy` crea el Worker,
sube el script y adjunta el binding D1 y el dominio — no hace falta crear
nada a mano en el panel de Cloudflare primero.

### 3 · Cargar las seis variables de ejecución como Secret *(una vez)*

Ver tabla arriba.

### 3b · Cabeceras de seguridad para los assets estáticos *(una vez)*

El Worker emite las seis cabeceras de seguridad (`routes/headers.go`) solo
en sus propias respuestas; el HTML del shell del panel lo sirve Cloudflare
directamente y no las lleva (ver `docs/ARCHITECTURE.md` §8.8). Configuralas
en Cloudflare (Transform Rules → Modify Response Header, o un archivo
`_headers` en `web/public/`) con los mismos valores:

| Cabecera | Valor |
|---|---|
| `Content-Security-Policy` | `default-src 'self'; script-src 'self' 'wasm-unsafe-eval'; style-src 'self'; img-src 'self' data: https://lh3.googleusercontent.com; connect-src 'self'; font-src 'self'; object-src 'none'; base-uri 'none'; form-action 'self'; frame-ancestors 'none'` |
| `X-Frame-Options` | `DENY` |
| `X-Content-Type-Options` | `nosniff` |
| `Referrer-Policy` | `strict-origin-when-cross-origin` |
| `Strict-Transport-Security` | `max-age=63072000; includeSubDomains` |
| `Permissions-Policy` | `camera=(), microphone=(), geolocation=(), payment=()` |

### 4 · Verificación

```bash
curl https://iam.velty.cl/api/health
# → {"ok":true}
```

### 5 · De aquí en adelante: nada manual

Cada push a `main` corre `go vet`, los tests, la migración de esquema D1
(`go run ./cmd/migrate`) y `goflare deploy`. La migración se ejecuta antes del
despliegue del Worker para asegurar que el esquema de base de datos esté
actualizado. Si los tests o la migración fallan **no se despliega**.

## Diagnóstico

| Síntoma | Causa |
|---|---|
| `Error: AccountID is required` | falta `CLOUDFLARE_ACCOUNT_ID` en GitHub Secrets |
| `This Worker does not exist on your account` al atar el dominio | el Worker todavía no se desplegó — el primer `goflare deploy` lo crea y lo ata en el mismo paso; no hay que atar el dominio a mano antes |
| El despliegue pasa pero `iam` no arranca | faltan una o más de las 6 variables de ejecución (paso 3) |
| El login falla con `redirect_uri_mismatch` | `GOOGLE_REDIRECT_URL` no coincide con la registrada en Google Cloud Console |
| Un consumidor recibe 403 en `/api/token` | `client_secret` incorrecto, proyecto no registrado o desactivado desde el panel — ver `config.CreateProject` |
