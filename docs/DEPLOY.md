# Despliegue de `iam` — el camino único

Mismo mecanismo que [`veltylabs/misitio`](https://github.com/veltylabs/misitio/blob/main/docs/DEPLOY.md):
GitHub Actions corre `goflare build` + `goflare deploy` (Go puro, sin Node ni
Wrangler — ver `AGENTS.md`). `iam` es un Worker con panel (assets estáticos +
`client.wasm` en `web/public`).

## Declaración tipada de despliegue (`config/deploy.go`)

El despliegue de `iam` se declara de forma tipada en `config/deploy.go` mediante
`deploy.Plan`.

Abre `iam` en la aplicación de escritorio `webtyp`, ve a la pestaña **DEPLOY** y
sigue la lista de comprobación guiada (checklist construida automáticamente desde
`config/deploy.go`). El cliente OAuth de Google es el único paso manual y la
lista de comprobación muestra sus pasos exactos.

Las variables y secretos de ejecución son inyectados al entorno del Worker por el sistema:
- `GOOGLE_CLIENT_ID`: Identificador del cliente OAuth en Google Cloud Console.
- `GOOGLE_CLIENT_SECRET`: Clave secreta del cliente OAuth.
- `GOOGLE_REDIRECT_URL`: `https://iam.velty.cl/oauth/callback/google`.
- `JWT_SECRET`: Clave simétrica que firma tokens de autorización.
- `IAM_PANEL_ORIGIN`: Origen exacto del panel (`https://iam.velty.cl`).
- `IAM_ADMIN_EMAILS`: Lista de correos semilla asignados al rol `iam_admin`.

La base de datos D1 (`velty-iam-db`) se resuelve automáticamente por nombre por `goflare`
y su binding `DB` es vinculado al Worker sin requerir configuración manual de IDs.

## Despliegue de la ola de seguridad

Para transicionar la infraestructura al esquema de aserciones de máquina (RFC 7523) y
passkeys para administración, sigue este orden estricto:

1. **Desplegar `iam`**:
   Desde la pestaña **DEPLOY** de `webtyp` (o mediante push a `main`).
2. **Generar el par de claves para cada consumidor**:
   Ejecuta localmente:
   ```bash
   go run ./cmd/keygen
   ```
   Esto imprime la clave privada en formato base64url y la clave pública SPKI en base64url.
3. **Declarar la clave privada en el consumidor**:
   La app consumidora declara `IAM_CLIENT_PRIVATE_KEY` en su propio `config/deploy.go`
   (`Source: deploy.Typed`, `To: []deploy.Place{deploy.ToRuntime}`). Su desarrollador
   ingresa la clave privada generada en la pestaña **DEPLOY** de ese consumidor (nunca
   en `.env`, un chat o tableros externos).
4. **Registrar la clave pública en el panel de `iam`**:
   Un administrador inicia sesión en `https://iam.velty.cl`. El primer administrador
   debe enrolar su passkey WebAuthn dentro de los primeros 5 minutos tras su login con Google.
   En la sección de proyectos, registra o actualiza la clave pública del consumidor.
5. **Desplegar el consumidor**.
6. **Limpieza en el consumidor**:
   El consumidor retira el antiguo `IAM_CLIENT_SECRET` de sus declaraciones y entorno.

> [!NOTE]
> Hasta completar el paso 4, las llamadas de máquina del consumidor a `/api/token`,
> `/api/users/resolve` y `/api/roles/assign` recibirán 403 Forbidden.

## Cabeceras de seguridad para assets estáticos

El Worker emite las cabeceras de seguridad en sus respuestas (`SecurityHeaders`). El HTML
estático del shell del panel servido por Cloudflare debe llevar los mismos valores
(configurados en Cloudflare Transform Rules o `_headers`):

| Cabecera | Valor |
|---|---|
| `Content-Security-Policy` | `default-src 'self'; script-src 'self' 'wasm-unsafe-eval'; style-src 'self'; img-src 'self' data: https://lh3.googleusercontent.com; connect-src 'self'; font-src 'self'; object-src 'none'; base-uri 'none'; form-action 'self'; frame-ancestors 'none'` |
| `X-Frame-Options` | `DENY` |
| `X-Content-Type-Options` | `nosniff` |
| `Referrer-Policy` | `strict-origin-when-cross-origin` |
| `Strict-Transport-Security` | `max-age=63072000; includeSubDomains` |
| `Permissions-Policy` | `camera=(), microphone=(), geolocation=(), payment=()` |

## Verificación

```bash
curl https://iam.velty.cl/api/health
# → {"ok":true}
```

Cada push a `main` ejecuta `gotest`, la migración de esquema D1 (`go run ./cmd/migrate`)
y `goflare deploy`.
