← [Etapa 1](PLAN_STAGE_1_HEALTH.md) | Etapa 2 de 3 | Siguiente → [Etapa 3](PLAN_STAGE_3_ACTION.md) | Índice: [PLAN.md](PLAN.md)

# Etapa 2 — Sacar del Worker lo que no tiene por qué estar ahí

El paquete `config/` se compila para **los dos objetivos**: el Worker (`edge/`) y
el servidor de desarrollo (`web/`). Cada import que se añade ahí sin `//go:build !wasm`
entra en el binario del edge. Dos cosas se colaron así.

Cifras medidas compilando de verdad, no estimadas:

| | crudo | gzip |
|---|---|---|
| hoy | 600.380 B | 212.539 B |
| tras la parte A | 586.388 B (**−13.992**) | 206.885 B |
| tras A y B | 570.517 B (**−29.863**) | 198.971 B |

---

## Parte A — Borrar `config/login.go`

**Tiene cero llamadores.** Compruébalo tú mismo antes de borrar:

```sh
grep -rn "LoginScreen" --include=*.go .
```

Solo aparece su propia definición. Nada la invoca.

Y arrastra al Worker una cadena de interfaz de usuario entera:

```
veltylabs/iam/edge → veltylabs/iam/config → tinywasm/components/actionbutton → tinywasm/dom
```

`tinywasm/dom` son 5.129 B de manipulación del DOM del navegador **dentro de un
Cloudflare Worker**, donde no hay DOM. Detrás vienen `layout/login`, `widget`,
`css`, `svg` y `html`.

### Qué hacer

1. `rm config/login.go`.
2. Comprueba que `github.com/tinywasm/components` y `github.com/tinywasm/layout`
   desaparecen del grafo del edge:
   ```sh
   GOOS=js GOARCH=wasm go list -deps ./edge | grep -E "components|layout|/dom"
   ```
   Debe salir vacío. Si no, algo más los importa: **encuentra qué y anótalo en
   el PR**, no borres a ciegas.
3. `go mod tidy`. Si `components` o `layout` siguen en `go.mod` es porque `web/`
   los usa, y eso está bien: `web/` es `!wasm`.

> La pantalla de login vive hoy en `tinywasm/layout/login`. Cuando iam tenga que
> servirla de verdad, se construye entonces, en el paquete de rutas que la sirva
> y detrás de un `//go:build !wasm` si corresponde. Guardarla sin usar en un
> paquete compartido no la acerca a existir; solo la mete en el Worker.

---

## Parte B — bcrypt fuera del camino caliente

### El problema, y por qué es mayor de lo que parece

[`config/projects.go`](../config/projects.go) usa bcrypt para el
`client_secret` de cada proyecto:

```go
hash, err := bcrypt.GenerateFromPassword([]byte(plainSecret), bcrypt.DefaultCost)
...
return bcrypt.CompareHashAndPassword([]byte(rows[0].ClientSecretHash), []byte(plainSecret)) == nil, nil
```

`VerifyProjectSecret` se llama desde **dos rutas del edge**
([`routes/routes.go`](../routes/routes.go), líneas 132 y 214). Y
`bcrypt.DefaultCost` es 10, o sea 2¹⁰ = 1024 rondas de Blowfish, **por diseño**.

Eso son ~50-100 ms de CPU pura por petición. El plan Free de Cloudflare Workers
da **10 ms de CPU** por petición. Aunque el plan sea de pago, es CPU facturable
gastada a propósito en cada llamada a `/api/token`.

El peso es lo de menos: `bcrypt` + `blowfish` son 15.871 B, de los cuales 4.194 B
son las tablas de Blowfish.

### Por qué HMAC es el reemplazo correcto, no un atajo

bcrypt existe para **frenar la fuerza bruta contra contraseñas humanas**, que
tienen poca entropía. Un factor de costo alto compra tiempo cuando el atacante
puede adivinar el secreto.

Un `client_secret` no es eso. Es una cadena aleatoria de alta entropía que genera
el servidor, para máquinas. No hay nada que adivinar, así que no hay nada que
frenar: el estiramiento de clave no compra seguridad, solo quema CPU. Es la
recomendación estándar para credenciales de máquina y es lo que hacen los
servidores OAuth.

Lo que sí hace falta es que el hash almacenado no sirva si se filtra la base, y
eso lo da el HMAC con una clave del servidor: sin la clave, la base robada no
permite calcular el hash de un secreto candidato.

> ⚠️ **Esto cambia una primitiva de seguridad.** Está razonado arriba y es la
> práctica estándar para secretos de máquina, pero **si algo de esto no te
> cuadra al implementarlo, detente y dilo en el PR** en vez de improvisar una
> variante.

### La clave: derivada, no reutilizada

`config.EnvJWTSecret` ya existe y ya está provisionado. **No lo uses tal cual**
como clave del HMAC: una misma clave para firmar tokens y para hashear secretos
viola la separación de claves, y un fallo en un uso compromete el otro.

Derívala:

```go
// projectSecretKeyLabel separa la clave que hashea client_secret de la que
// firma JWT. Reutilizar una sola clave para dos propositos hace que un fallo en
// uno comprometa el otro; derivar cuesta un HMAC y elimina el acoplamiento.
// El sufijo de version permite rotar el esquema sin ambiguedad.
const projectSecretKeyLabel = "iam:project-secret:v1"

func projectSecretKey() []byte {
	return hmac.HMACSHA256([]byte(env.Get(EnvJWTSecret)), []byte(projectSecretKeyLabel))
}
```

### El nuevo `config/projects.go`

Sustituye el import `github.com/tinywasm/crypto/bcrypt` por
`github.com/tinywasm/crypto/hmac` y `github.com/tinywasm/base64` — **ambos ya
están en el binario**, así que no añaden peso. La API disponible es:

```go
func hmac.HMACSHA256(key, message []byte) []byte
func hmac.HMACEqual(mac1, mac2 []byte) bool     // comparacion en tiempo constante
func base64.URLEncode(src []byte) string
func base64.URLDecode(s string) ([]byte, error)
```

```go
// hashProjectSecret deriva el hash almacenable de un client_secret. Es HMAC y
// no bcrypt a proposito: un client_secret es aleatorio y de alta entropia, asi
// que no hay nada que frenar con un factor de costo — solo CPU que quemar, y en
// un Worker eso se paga en cada peticion contra un limite de 10 ms.
func hashProjectSecret(plainSecret string) string {
	return base64.URLEncode(hmac.HMACSHA256(projectSecretKey(), []byte(plainSecret)))
}
```

`CreateProject` guarda `hashProjectSecret(plainSecret)`.

`VerifyProjectSecret` compara con `hmac.HMACEqual` sobre los bytes decodificados
— **nunca con `==`**, y nunca comparando las cadenas en base64:

```go
	stored, err := base64.URLDecode(rows[0].ClientSecretHash)
	if err != nil {
		return false, nil
	}
	return hmac.HMACEqual(stored, hmac.HMACSHA256(projectSecretKey(), []byte(plainSecret))), nil
```

Un `ClientSecretHash` que no decodifica devuelve `(false, nil)`, no un error: es
una fila corrupta o de formato viejo, y desde fuera eso es exactamente lo mismo
que un secreto incorrecto. Devolver un error distinguible convierte el formato
del hash en un oráculo.

Actualiza el comentario de `VerifyProjectSecret`, que hoy dice "vía bcrypt".

### Los datos: no hay que migrar nada

```sql
SELECT COUNT(*) FROM project;  -- 0
```

La tabla está vacía. **No escribas código de migración ni de doble formato.** Si
al implementar encuentras filas, detente y repórtalo: significaría que alguien
provisionó proyectos entre la escritura de este plan y su ejecución, y esa
decisión es del humano.

---

## Criterios de aceptación

- `grep -rn "LoginScreen" .` → vacío.
- `grep -rn "bcrypt" .` → vacío en todo el repo, `go.mod` incluido.
- `GOOS=js GOARCH=wasm go list -deps ./edge | grep -cE "components|layout|/dom|blowfish"` → `0`.
- El binario del edge baja de 600.380 B. Mídelo y **pon la cifra en el PR**:
  ```sh
  cd edge && GOOS=js GOARCH=wasm tinygo build -target wasm -no-debug -o /tmp/edge.wasm main.go
  stat -c%s /tmp/edge.wasm
  ```
  Esperado: alrededor de 570.500 B. Si sale muy por encima, algo no se fue.
- `go build ./...`, `go vet ./...`, `GOOS=js GOARCH=wasm go vet ./edge/...` limpios.
- `go test ./tests/...` en verde.

## Tests — en `tests/`

1. `TestProjectSecretRoundTrip` — `CreateProject` con un secreto conocido y
   después `VerifyProjectSecret` con el mismo secreto → `true`; con otro →
   `false`. Necesita `JWT_SECRET` en el entorno: usa `t.Setenv`.
2. `TestProjectSecretRejectsCorruptHash` — escribe una fila con
   `ClientSecretHash` que no es base64 válido; `VerifyProjectSecret` devuelve
   `(false, nil)`, **no** un error.
3. `TestProjectSecretKeyIsDerivedNotReused` — `projectSecretKey()` no es igual a
   `[]byte(env.Get(EnvJWTSecret))`. Es la aserción que impide que alguien
   "simplifique" la derivación más adelante.

## Lo que NO hay que hacer

- **No** toques `tinywasm/crypto`. Su árbol de stdlib es una decisión de
  arquitectura tomada y documentada aguas arriba; no es una fuga.
- **No** quites bcrypt de `tinywasm/crypto`. Sigue siendo lo correcto para
  contraseñas humanas y otros consumidores lo usan. Lo que cambia es que **iam**
  deja de usarlo para un secreto de máquina.
- **No** añadas un `//go:build !wasm` a `config/projects.go` para "sacarlo del
  edge". `VerifyProjectSecret` se llama desde rutas del edge; ese tag rompería
  la compilación del Worker.
