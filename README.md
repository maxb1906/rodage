# Rodage

**Testeur de charge et de stabilité matériel portable** : valider une machine neuve, reconditionnée ou instable avant de s'en servir.
Windows et Linux, x64 et ARM64. Un seul binaire, aucun pilote embarqué, aucune télémétrie. Licence GPL-3.0.

*English below.*

## Ce que fait Rodage

| Composant | Tests |
|---|---|
| **Processeur** | Calculs à résultat connu (noyaux SIMD SSE2 / AVX2 / AVX-512 / NEON, FFT, produit de matrices, SHA-256), un fil épinglé par cœur logique : une erreur silencieuse est détectée et attribuée à un cœur précis. Détection du bridage (chute de fréquence sous charge) et de sa cause quand les sondes le permettent. |
| **Mémoire vive** | 87 % de la RAM libre, verrouillée (`mlock` / `VirtualLock`). Motifs : adresse dans la cellule, walking ones/zeros, moving inversions (fixes et aléatoires). Compteurs ECC (EDAC sous Linux, WHEA sous Windows). |
| **Disque** | Mode fichier sans risque (`O_DIRECT` / `FILE_FLAG_NO_BUFFERING`) : séquentiel 1 Mo, aléatoire 4K QD1 / QD32, latences p99, chute après saturation du cache SLC, relecture vérifiée de chaque bloc. SMART / santé NVMe avant et après. Plafond d'écriture pour préserver l'endurance. Mode brut destructif optionnel (voir plus bas). |
| **Réseau** | Protocole maison entre deux machines Rodage : TCP multi-flux et UDP (pertes, gigue) dans les deux sens, stabilité du lien (renégociations, erreurs de la carte). |
| **Sûreté** | Arrêt d'urgence au-delà de TjMax − marge, sur batterie basse ou secteur débranché, et par watchdog si un test se fige. Un plantage complet est signalé au lancement suivant. |

Deux modes :

- **Bench** : court (≈ 2 min), reproductible, produit des chiffres comparables ; ils alimentent une base locale de références par modèle de CPU et de disque.
- **Stress (rodage)** : long, verdict *réussi / réussi avec réserves / échoué / incomplet*. Profils Rapide (~10 min), Standard (~1 h), Rodage complet (4 à 12 h), Personnalisé.

Chaque test produit un **rapport de validation** JSON + HTML + PDF : numéro de série SMBIOS, configuration détectée, tests passés, niveau de confiance des sondes, courbes, verdict et limites du test. Deux rapports se comparent (avant / après).

## Utilisation

### Interface graphique

Lancez `rodage` (Linux) ou `rodage-amd64.exe` / `rodage-arm64.exe` (Windows). Choisissez Bench ou Stress, la durée, les composants, puis **Lancer le test**. Les réglages par défaut sont sûrs. Les rapports sont rangés dans le dossier de configuration de l'utilisateur (`~/.config/rodage/reports`, `%AppData%\rodage\reports`).

### Ligne de commande

```sh
rodage-cli info                                  # configuration, sondes, disques
rodage-cli bench                                 # mesures courtes
rodage-cli stress --profile quick                # rodage de 10 min
rodage-cli stress --profile full --duration 10h  # rodage complet
rodage-cli stress --profile custom --duration 45m --tests cpu,ram
rodage-cli stress --net 192.168.1.20             # ajoute le test réseau
rodage-cli server                                # sur la seconde machine (port 7420 TCP+UDP)
rodage-cli report rapport.json --lang en         # régénère HTML et PDF
rodage-cli compare avant.json apres.json --html diff.html
rodage-cli refs list | export f.json | import f.json
```

Codes de sortie : `0` réussi, `1` échoué, `2` incomplet, `3` erreur d'utilisation. `--json` diffuse les événements en JSON pour l'automatisation.

Options utiles : `--ram-size 8G`, `--disk-dir D:\test`, `--write-cap 100G`, `--margin 8`, `--kernel avx2`, `--allow-battery`, `--lang fr|en`, `--out dossier`.

### Mode disque brut (destructif)

```sh
rodage-cli disks                                         # liste et éligibilité
rodage-cli stress --profile standard --tests disk --disk-raw nvme1n1 --confirm-serial S5GXNX0R123456
```

Garde-fous : disque sans partition, non monté, jamais le disque système, ouverture exclusive, confirmation par saisie exacte du numéro de série. **Tout le contenu du disque est effacé.** Droits administrateur requis.

### Sondes et niveaux de mesure

1. **Windows sans pilote** : fréquence effective (compteurs de performance), « % Performance Limit », zones thermiques ACPI. Le bridage est détecté, sa cause exacte reste inconnue.
2. **Windows + [PawnIO](https://pawnio.eu)** : si le pilote PawnIO est installé et le module `IntelMSR.bin` placé à côté de l'exécutable (ou dans le dossier de PawnIO), Rodage lit les MSR : températures par cœur, cause du bridage (PL1/PL2, thermique, VRM), puissance.
3. **Linux** : `hwmon` (coretemp, k10temp…), `cpufreq`, compteurs `thermal_throttle` et, en root sur Intel, `/dev/cpu/*/msr` (`modprobe msr`).

Sans droits administrateur, Rodage fonctionne mais signale ce qui est limité : SMART, MSR et mode brut.

### Limites connues

- Le test mémoire porte sur des adresses virtuelles et sur la mémoire allouable : il n'est pas exhaustif. Une image amorçable est prévue plus tard.
- Les causes exactes du bridage ne sont décodées que sur Intel (MSR). Sur AMD, la chute de fréquence est détectée et la cause reste « probable ».
- Version postes de travail et portables ; serveurs, GPU, macOS et compatibilité iperf3 sont prévus plus tard.

## Compilation

Prérequis : Go ≥ 1.26, Node.js, [Wails v2](https://wails.io) (`go install github.com/wailsapp/wails/v2/cmd/wails@latest`). Pas de cgo pour le moteur ni pour la CLI.

```sh
go build ./cmd/rodage-cli                                # CLI (toutes plateformes : GOOS/GOARCH)
wails build -tags webkit2_41                             # GUI Linux (WebKitGTK 4.1)
wails build -platform windows/amd64,windows/arm64        # GUI Windows (WebView2)
go test ./...                                            # tests (stress ≤ 60 s, RAM ≤ 1 Go)
```

Les noyaux amd64 sont générés avec [avo](https://github.com/mmcloughlin/avo) :

```sh
go generate ./internal/cpu/kernels     # régénère kernels_amd64.s depuis asm/gen
```

Les noyaux NEON (`internal/cpu/kernels/kernels_arm64.s`) sont écrits à la main. Chaque noyau a un équivalent Go pur (`kernels_go.go`) ; les tests vérifient l'identité au bit près. Le tag `purego` force le Go pur.

## Architecture

```
cmd/rodage-cli      CLI (automatisation, mode serveur)
app.go, main.go     GUI Wails (liaison seulement)
frontend/           interface (TypeScript, sans framework)
internal/
  engine            orchestration : profils, phases, sûreté, rapport
  cpu, cpu/kernels  charges vérifiées, épinglage, bench ; noyaux SIMD
  memtest           test mémoire verrouillé, ECC
  disk              E/S directes, SMART/NVMe, mode brut
  netbench          client/serveur TCP/UDP, stabilité du lien
  sensors           sondes (hwmon, MSR, PDH, PawnIO), throttling
  safety            arrêt d'urgence, watchdog, marqueur de plantage
  report            verdict, HTML, PDF, comparaison
  refdb             références locales
  sysinfo, i18n     détection système, textes FR/EN
asm/gen             générateur avo (module séparé)
```

Le moteur (`internal/`) ne dépend jamais de Wails : la CLI et la GUI appellent le même code.

---

## English

**Rodage** is a portable hardware load and stability tester for Windows and Linux (x64 and ARM64): validate a new, refurbished or unstable machine before relying on it. Single binary, no bundled kernel driver, no telemetry, GPL-3.0.

- **CPU**: verified computations with known results (SSE2/AVX2/AVX-512/NEON SIMD kernels, FFT, matrix multiply, SHA-256), one thread pinned per logical core, so a silent error is caught and pinned to a specific core. Throttling detection, plus its cause when the sensors allow it.
- **Memory**: 87% of free RAM, locked; address-in-cell, walking bits, moving inversions; ECC counters (EDAC / WHEA).
- **Disk**: safe file mode with direct I/O: 1 MB sequential, 4K QD1/QD32 random, p99 latency, SLC cache drop, read-back verification, SMART/NVMe health before and after, write cap. Optional destructive raw mode with strict safeguards.
- **Network**: custom protocol between two Rodage instances (`rodage-cli server`): multi-stream TCP and UDP loss/jitter in both directions, link stability.
- **Safety**: emergency stop at TjMax − margin, on low battery or unplugged charger, and by a watchdog when a test hangs.
- **Reports**: JSON + HTML + PDF with SMBIOS serial, detected configuration, sensor confidence level, charts and a plain-language verdict; before/after comparison; local reference database.

Run `rodage-cli help` for the commands, or start the GUI. See the French sections above for build instructions and details; command-line flags and outputs are the same in both languages (`--lang en`).
