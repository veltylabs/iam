# Despliegue de `iam` — el camino único

Mismo mecanismo que [`veltylabs/misitio`](https://github.com/veltylabs/misitio/blob/main/docs/DEPLOY.md):
GitHub Actions corre `goflare build` + `goflare deploy` (Go puro, sin Node ni
Wrangler — ver `AGENTS.md`). La diferencia con `misitio`: `iam` es un Worker
API puro, sin `web/public`, así que el deploy nunca toca el protocolo de
subida de assets — solo sube el script (`edge.js`/`edge.wasm`) y el binding
D1.

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

### De ejecución — variables del Worker en Cloudflare (4)

El Worker las lee con `env.Reader` (`cloudflare.Env` en `edge/main.go`, ver
`config/auth.go`). **Sin las tres de Google y sin `JWT_SECRET` no arranca** —
deliberado: un servicio de identidad que arranca a medias es peor que uno que
no arranca.

Cloudflare → Workers & Pages → `iam` → *Settings* → *Variables and Secrets*,
las cuatro como tipo **Secret** (no texto plano — un `PUT` de script que no
declara un binding `secret_text` en su metadata lo borra si era texto plano,
pero lo conserva si era Secret):

| Variable | Obligatoria | Qué pasa si falta |
|---|---|---|
| `GOOGLE_CLIENT_ID` | sí | el Worker no arranca |
| `GOOGLE_CLIENT_SECRET` | sí | el Worker no arranca |
| `GOOGLE_REDIRECT_URL` | sí | `https://iam.velty.cl/oauth/callback/google`, registrada en Google Cloud Console |
| `JWT_SECRET` | sí | el Worker no arranca — firma la cookie SSO y los tokens de autorización |

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

### 3 · Cargar las cuatro variables de ejecución como Secret *(una vez)*

Ver tabla arriba.

### 4 · Verificación

```bash
curl https://iam.velty.cl/api/health
# → {"ok":true}
```

### 5 · De aquí en adelante: nada manual

Cada push a `main` corre `go vet`, los tests y `goflare deploy`. Si los tests
fallan **no se despliega**.

## Diagnóstico

| Síntoma | Causa |
|---|---|
| `Error: AccountID is required` | falta `CLOUDFLARE_ACCOUNT_ID` en GitHub Secrets |
| `This Worker does not exist on your account` al atar el dominio | el Worker todavía no se desplegó — el primer `goflare deploy` lo crea y lo ata en el mismo paso; no hay que atar el dominio a mano antes |
| El despliegue pasa pero `iam` no arranca | faltan una o más de las 4 variables de ejecución (paso 3) |
| El login falla con `redirect_uri_mismatch` | `GOOGLE_REDIRECT_URL` no coincide con la registrada en Google Cloud Console |
| Un consumidor recibe 403 en `/api/token` | `client_secret` incorrecto o proyecto no registrado — ver `config.CreateProject` |
