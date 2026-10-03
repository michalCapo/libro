# Vzdialený backend cez TLS párovanie (možnosť B)

Cieľ: Libro backend beží na serveri, ovláda sa z Libra na notebooku. Rovnaké UI ako dnes. Jeden frontend vie pracovať s viacerými backendmi. Celá implementácia je v Libre, bez Vemari, bez relay, bez frp.

Stav: návrh. `--listen`, `pair`, `devices`, proxy `app-N.localhost` ani TLS listener zatiaľ neexistujú.

## Hlavná myšlienka

Žiadny vlastný protokol ani multiplexing. Každé TCP spojenie z notebooku = jedno TLS spojenie na server. Server na TLS listeneri obsluhuje **ten istý `http.Handler`** ako dnes, obalený autorizačnou vrstvou.

```
Electron ──http + lokálny token──▶ 127.0.0.1:82xx (Libro na notebooku, pipe)
         ══ TLS 1.3 + certifikát zariadenia ══▶ server (Libro backend)
                                                  └─ autorizácia → rovnaký handler ako :8100
                                                     └─ app-3000.localhost → 127.0.0.1:3000
```

- WebSockety (UI, terminály, HMR) idú cez pipe bez zmeny.
- Pipe zachová `Host` aj `Origin`. g-sui porovnáva ich hostiteľskú časť, takže UI funguje.
- Žiadna nová závislosť: `crypto/tls`, `crypto/x509`, `io.Copy`.
- Prehliadač drží spojenia otvorené (keep-alive, WebSocket), handshake na spojenie nevadí.
- Lokálne porty sú z rozsahu `8200+`. `8101` už používajú pomenované inštancie (`electron/main.js:42`).

## Hranice dôvery

| Hranica | Kto ju prekračuje | Kontrola |
|---|---|---|
| Sieť → server | ktokoľvek na sieti | TLS 1.3, certifikát klienta povinný, pin zariadenia |
| Nespárované zariadenie → server | nový notebook | iba presný `POST /pair` s platným tokenom |
| Lokálny proces → pipe na notebooku | akýkoľvek program na notebooku | lokálny token, presný `Host`/`Origin` |
| Electron okno → backend | okno daného backendu | token patrí iba oknu toho backendu |
| Relácia → terminál | UI relácia | vlastníctvo terminálu, nie iba „PTY beží“ |
| Backend → aplikácia (proxy) | spárované zariadenie | register aplikácií spustených Libro |

Poradie spracovania požiadavky na serveri:

1. TLS handshake. Bez certifikátu klienta odmietnuť.
2. `VerifyConnection`: odtlačok certifikátu. Neznámy certifikát označiť ako nespárovaný. Táto kontrola beží aj pri TLS resumption, `VerifyPeerCertificate` nie.
3. Autorizačný handler obalí **celý** mux (stránky, assets, `/__ws`, terminály, notes, voice, proxy). Middleware g-sui pre stránky nestačí.
4. Nespárovaný certifikát: iba `POST /pair`, inak 403.
5. Podľa `Host`: `app-N.localhost` ide do proxy, inak do Libro UI. Neznámy `app-*` host vráti 404, nesmie spadnúť do Libro UI.

## Server (backend)

- Spustenie: `libro --no-desktop --listen <adresa:port>`. Port je voliteľný.
- Hlavný HTTP listener sa zmení z `":"+Port()` na `127.0.0.1` (`internal/app.go:487`). Toto treba opraviť aj bez remote režimu.
- Pri prvom štarte vygeneruje ECDSA kľúč a self-signed certifikát.
- Kľúče sú v súkromnom adresári `0700`, súbory `0600`. Na Windows obmedziť ACL na používateľa. Dnes sa dátový adresár vytvára s `0755` (`internal/db.go:50`).
- Limity: handshake timeout, max. veľkosť tela `/pair`, max. počet súbežných spojení, limit spojení na IP.
- Spojenia sa evidujú podľa ID zariadenia, aj po WebSocket hijacku. Pri odvolaní sa dajú zatvoriť.

## Párovanie

1. Na serveri `libro pair` vypíše kód:
   ```
   libro://server.example:8443#fp=<sha256 certifikátu servera>&t=<token>
   ```
   - token: aspoň 256 náhodných bitov, v DB iba hash, expirácia 10 minút na serveri;
   - kód sa prenáša dôveryhodnou cestou (terminál cez SSH, osobne). Kto podvrhne celý kód, nasmeruje notebook na seba.
2. Na notebooku: Settings → Backends → Pridať → vložiť kód. Token sa nikdy neloguje ani neposiela v URL navigácii.
3. Notebook vygeneruje kľúč a certifikát zariadenia, pripojí sa a **najprv** overí `fp` servera. Až potom pošle `POST /pair` s tokenom a menom zariadenia.
4. Server v jednej DB transakcii overí a spotrebuje token a uloží zariadenie. Dve súbežné použitia tokenu nemôžu uspieť obe. Odtlačok zariadenia berie z TLS spojenia, nie z tela požiadavky.
5. Rate limit `/pair`: globálny aj podľa IP. Limit podľa certifikátu nestačí, nový certifikát si útočník vygeneruje kedykoľvek.

Zmenený pin servera sa nikdy neprijme automaticky. Notebook ukáže chybu a vyžaduje nové párovanie.

## Zariadenia a odvolanie

- Zariadenie má stabilné ID. Meno je iba popis a môže sa opakovať.
- `libro devices` vypíše ID, meno, dátum párovania a posledné použitie.
- `libro devices remove <id>`:
  - zmaže zariadenie z DB;
  - oznámi to bežiacemu serveru cez lokálny autentifikovaný endpoint;
  - server zatvorí všetky spojenia toho zariadenia vrátane WebSocketov.
- Oprávnenie sa kontroluje pri každej HTTP požiadavke, nie iba pri handshake.
- Prvá verzia vypne TLS session resumption. Pridať ju neskôr spolu s kontrolou cez `VerifyConnection`.
- Strata kľúča notebooku: odvolať zariadenie, spárovať znova. Rotácia certifikátu servera: nový `fp`, všetky notebooky párujú znova.

## Notebook (frontend)

- Notebook vlastní zoznam backendov a kľúče zariadení. Server vlastní zariadenia a pair tokeny.
- Go proces Libra otvorí pre každý backend listener na `127.0.0.1` v rozsahu `8200+`. Port sa uloží k backendu, aby origin zostal stabilný (localStorage, cookies).
- **Lokálna autentifikácia:** `127.0.0.1` nie je ochrana. Pipe pustí iba požiadavky s lokálnym tokenom a presným `Host`/`Origin` (`localhost:82xx` alebo `app-N.localhost:82xx`). Token pridáva dôveryhodná časť Electronu (hlavný proces, `session.webRequest`), iba oknu a webview daného backendu. Token sa nikdy nepošle na server ani do aplikácie.
- Pri každom spojení: TLS dial s certifikátom zariadenia, overenie `fp`, potom `io.Copy` oboma smermi. Pipe nikdy automaticky neopakuje rozpracované požiadavky.
- Keď server nie je dostupný: lokálna stránka „odpojené“ (aj keď sa vzdialené UI ešte nenačítalo), opakovanie s backoffom.
- Lokálne Settings (zoznam backendov) fungujú aj bez dostupného backendu.

## Electron

Dnes je jedno globálne okno, jeden `serverURL` (`electron/main.js:42`), globálne IPC a zatvorenie okna ukončí aplikáciu. Treba:

- mapu backend → okno;
- IPC smerovať podľa odosielateľa (`event.sender`), nie na globálne okno;
- zatvorenie okna backendu iba odpojí, neukončí aplikáciu;
- partition okna, webview aj popupov obsahuje stabilné ID backendu. Dnes sa partition webview odvodzuje z názvu projektu (`internal/browser_profile.go`), takže dva backendy s rovnakým projektom by zdieľali cookies;
- partition nastaviť pred prvou navigáciou;
- kontrolu kompatibility verzie backendu s Electron preloadom (prevziať z `docs/remote.html`).

## Relácie a terminály

- ID relácie je dnes predvídateľné (`session-N`, `internal/state.go:103`) a nie je to bezpečnostná identita.
- Väzba: zariadenie → relácia → terminál. Akcia s cudzím SID sa odmietne.
- Terminálový endpoint dnes pustí klienta k živému PTY bez kontroly vlastníctva (`internal/components/terminal.go:687`). Opraviť: vlastníctvo kontrolovať vždy.
- Prvá verzia: jeden aktívny GUI klient na backend. Ďalší dostane „obsadené“ s možnosťou prevziať reláciu.
- Zatvorenie okna (`app.close.all`, `internal/app.go:1404`) dnes zastaví všetky terminály. Oddeliť „odpojiť okno“ od „zastaviť runtime“.
- g-sui po výpadku dlhšom ako ~15 s obnoví stránku a `GET /` (`internal/app.go:1493`) vytvorí novú reláciu. Treba obnoviť existujúcu reláciu zariadenia.

## Aplikácie v browser paneloch

Backend robí reverzný proxy podľa subdomény:

```
http://app-3000.localhost:82xx  →  pipe  →  backend  →  http://127.0.0.1:3000
```

- `*.localhost` Chromium resolvuje na loopback (`127.0.0.1` aj `::1`). Lokálny listener musí počúvať na oboch, alebo overiť správanie v dodávanom Electrone.
- Každá aplikácia má vlastný origin, oddelený od Libra.
- `httputil.ReverseProxy` podporuje WebSocket upgrade.

**Povolené ciele:**

- iba register aplikácií, ktoré Libro spustilo a ešte bežia. Pridelený port nestačí, príkaz ho môže ignorovať (`internal/application_control.go:277`);
- cieľ je vždy `127.0.0.1`, nikdy iná adresa;
- zakázané sú porty Libra a lokálne control porty;
- v remote režime je port takeover vypnutý (`internal/application_settings.go:118`). Obsadený port = chyba.

**Hlavičky:**

- `Host` sa prepíše na `localhost:N`. `Origin` zostane pôvodný, čo môže rozbiť CSRF alebo WebSocket kontroly aplikácie. Aplikácia dostane `X-Forwarded-Host` a `X-Forwarded-Proto: http` (Electron používa HTTP, interné TLS nemení schému).
- `Location` sa prepíše iba z `localhost:N` / `127.0.0.1:N` na `app-N.localhost:82xx`. Cudzie adresy (OAuth) zostanú bez zmeny.
- Libro lokálny token ani Libro cookies sa nikdy neposielajú aplikácii.

**Cookies a HMR:**

- host-only cookies fungujú. `Domain`, `Secure` a `SameSite` treba otestovať;
- HMR musí ísť cez verejný host a port proxy. Pri Vite nastaviť `server.hmr.clientPort` / `host`, inak sa klient pripája priamo na port aplikácie.

**URL aplikácie:**

- server nepozná lokálny port notebooku a rôzne notebooky majú rôzne porty;
- backend vracia iba ID a port aplikácie. URL poskladá frontend z vlastného `location`;
- verejné URL sa neukladajú do serverovej DB (`internal/application_settings.go:86`).

Nefunguje pre aplikácie s natvrdo `localhost:N` v HTML/JS, CORS alebo OAuth callbacku.

## CLI a MCP na serveri

Bridge (`internal/agent_control.go`) číta lokálny descriptor a volá Electron na notebooku. Na serveri bez Electronu nefunguje. Vzdialení agenti preto potrebujú:

- obsluhu `notes`, `application`, `children` a MCP priamo v backende;
- lokálny autentifikovaný endpoint na serveri (rovnaký použije `libro devices remove`);
- zachovanie dnešných obmedzení na workspace.

Toto je súčasť prvej verzie, nie neskorší doplnok.

## Úpravy v Libre

| Súbor | Zmena |
|---|---|
| `main.go` | `--listen`, príkazy `pair`, `devices` |
| `internal/app.go` | bind `127.0.0.1`, TLS listener, autorizačná vrstva, proxy, odpojenie vs. zastavenie |
| `internal/remote_server.go` (nový) | certifikát, `VerifyConnection`, `/pair`, register spojení, odvolanie |
| `internal/remote_client.go` (nový) | lokálne listenery, lokálny token, TLS pipe, reconnect |
| `internal/remote_proxy.go` (nový) | proxy `app-N.localhost`, register povolených aplikácií |
| `internal/db.go` | `0700` dátový adresár; tabuľky `remote_devices`, `pair_tokens` (server), `backends` (notebook) |
| `internal/state.go` | väzba zariadenie → relácia, obnovenie relácie |
| `internal/components/terminal.go` | vždy kontrolovať vlastníctvo terminálu |
| `internal/browser_profile.go` | ID backendu v partition |
| `internal/application_settings.go`, `application_port.go` | vypnutý port takeover v remote režime, URL skladá frontend |
| `internal/agent_control.go` | CLI/MCP priamo v backende |
| `internal/desktop.go`, `electron/main.js`, `electron/preload.js` | okná podľa backendu, IPC podľa odosielateľa, lokálny token, partitions, kontrola verzie |
| Settings UI | zoznam backendov, pridať, odobrať, stav pripojenia |

## Vzťah k `docs/remote.html`

| Starší návrh | Možnosť B |
|---|---|
| jedno spoločné okno | okno na backend (neskôr možno spojiť) |
| jeden GUI klient | zachované pre prvú verziu |
| SSH inštalácia a reuse backendu | odložené; backend sa spúšťa ručne |
| kontrola kompatibility verzií | zachované |
| `doctor` | odložené |
| manuálne port forwardingy | nahradené proxy `app-N.localhost` |

Jednoduchšia alternatíva: backend spustený ručne a systémové SSH (`ssh -L`). Lokálnu autentifikáciu na notebooku potrebuje aj tá.

## Čo tento návrh nerieši

- Výstup terminálu počas odpojenia sa po reconnecte neprehrá (iba 64 KiB log).
- Prílohy z page-tool sa ukladajú na notebooku, vzdialený agent ich nevidí. Treba upload na server.
- Agent CLI a ich auto-update (dnes v Electrone, `electron/agent-updates.js`) musia bežať na serveri.
- Viac používateľov na jednom backende.
- Server musí byť sieťovo dostupný z notebooku. Za NAT na oboch stranách to nepomôže.

## Poradie

1. Bezpečnostný základ: bind `127.0.0.1`, kontrola vlastníctva terminálu, vypnutý port takeover, `0700` dátový adresár.
2. CLI/MCP priamo v backende, oddelenie „odpojiť okno“ od „zastaviť runtime“, obnovenie relácie.
3. TLS listener, autorizačná vrstva, párovanie, odvolanie.
4. Notebook: pipe, lokálny token, Electron okná a partitions podľa backendu.
5. Proxy `app-N.localhost` s registrom aplikácií.
6. Settings UI pre backendy.

## Akceptačné scenáre

- Dve súbežné `POST /pair` s rovnakým tokenom: uspeje práve jedna.
- Expirovaný alebo použitý token: 403.
- Nespárovaný certifikát: 403 na každú routu okrem `/pair` (stránky, assets, `/__ws`, terminály, notes, voice, proxy).
- Bez certifikátu: handshake zlyhá.
- Zmenený `fp` servera: notebook odmietne spojenie.
- Odvolanie zariadenia: otvorený WebSocket aj terminál sa zatvoria do 1 s. Nové spojenie zlyhá.
- Lokálny proces bez tokenu na `127.0.0.1:82xx`: odmietnutý.
- Terminál inej relácie: 403, aj keď PTY beží.
- Dva backendy s rovnakým názvom projektu: oddelené cookies a localStorage.
- Výpadok siete nad 15 s: po návrate rovnaká relácia, terminály bežia.
- Zatvorenie okna backendu: terminály na serveri bežia ďalej.
- `libro application` a MCP na serveri bez Electronu: fungujú.
- Proxy: neznámy port, port Libra a port aplikácie, ktorá už skončila, vrátia 404.
- Proxy: redirect na `localhost:N` prepísaný, redirect na cudzí OAuth server nezmenený.
- Proxy: Vite HMR funguje cez `app-N.localhost`.
- Proxy: Libro token ani cookies sa nedostanú do aplikácie.
