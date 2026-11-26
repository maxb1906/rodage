package i18n

// catalog : clé → {français, anglais}.
var catalog = map[string][2]string{
	// Verdicts
	"verdict.pass":            {"Réussi", "Passed"},
	"verdict.warn":            {"Réussi avec réserves", "Passed with warnings"},
	"verdict.fail":            {"Échoué", "Failed"},
	"verdict.incomplete":      {"Incomplet", "Incomplete"},
	"verdict.pass.long":       {"Aucune erreur détectée. La machine est stable dans les conditions testées.", "No errors detected. The machine is stable under the tested conditions."},
	"verdict.warn.long":       {"Aucune erreur de calcul ni de données, mais certains points méritent votre attention.", "No computation or data errors, but some points deserve your attention."},
	"verdict.fail.long":       {"Des erreurs ont été détectées : la machine n'est pas fiable en l'état.", "Errors were detected: the machine is not reliable as it stands."},
	"verdict.incomplete.long": {"Le test n'a pas pu aller au bout : le verdict ne porte que sur ce qui a été testé.", "The test could not complete: the verdict only covers what was tested."},

	// Modes et profils
	"mode.bench":       {"Bench", "Bench"},
	"mode.stress":      {"Stress (rodage)", "Stress (burn-in)"},
	"profile.quick":    {"Rapide (~10 min)", "Quick (~10 min)"},
	"profile.standard": {"Standard (~1 h)", "Standard (~1 h)"},
	"profile.full":     {"Rodage complet", "Full burn-in"},
	"profile.custom":   {"Personnalisé", "Custom"},
	"profile.bench":    {"Bench", "Bench"},

	// Composants
	"comp.cpu":    {"Processeur", "Processor"},
	"comp.ram":    {"Mémoire vive", "Memory"},
	"comp.disk":   {"Disque", "Disk"},
	"comp.net":    {"Réseau", "Network"},
	"comp.safety": {"Sûreté", "Safety"},
	"comp.system": {"Système", "System"},

	// Constats
	"f.cpu.ok":               {"%d cœurs logiques testés pendant %s, aucune erreur de calcul (%s).", "%d logical cores tested for %s, no computation errors (%s)."},
	"f.cpu.bench":            {"%d mesures, tous les calculs vérifiés sont exacts (%s).", "%d measurements, all verified computations are correct (%s)."},
	"f.cpu.errors":           {"%d erreur(s) de calcul silencieuse(s) sur le(s) cœur(s) %s : processeur instable (overclocking, tension, refroidissement ou défaut matériel).", "%d silent computation error(s) on core(s) %s: unstable processor (overclocking, voltage, cooling or hardware defect)."},
	"f.cpu.stalled":          {"Le(s) cœur(s) %s ont cessé de progresser (gel).", "Core(s) %s stopped making progress (hang)."},
	"f.cpu.nopin":            {"Épinglage par cœur indisponible : un cœur défaillant ne peut pas être isolé précisément.", "Per-core pinning unavailable: a faulty core cannot be isolated precisely."},
	"f.throttle":             {"Bridage détecté : la fréquence chute de %.0f %% sous charge (%.0f → %.0f MHz). Cause : %s.", "Throttling detected: frequency drops by %.0f%% under load (%.0f → %.0f MHz). Cause: %s."},
	"f.throttle.none":        {"Pas de bridage : fréquence stable sous charge (~%.0f MHz).", "No throttling: stable frequency under load (~%.0f MHz)."},
	"f.throttle.nofreq":      {"Fréquence non mesurable sur cette machine : bridage non vérifié.", "Frequency not measurable on this machine: throttling not checked."},
	"f.temp.max":             {"Température maximale du processeur : %.0f °C (seuil d'arrêt %.0f °C).", "Maximum processor temperature: %.0f °C (shutdown threshold %.0f °C)."},
	"f.temp.none":            {"Température du processeur non disponible sur cette machine.", "Processor temperature not available on this machine."},
	"cause.thermal":          {"thermique", "thermal"},
	"cause.prochot":          {"PROCHOT (signal de surchauffe externe)", "PROCHOT (external overheat signal)"},
	"cause.pl1":              {"limite de puissance longue durée (PL1)", "long-duration power limit (PL1)"},
	"cause.pl2":              {"limite de puissance courte durée (PL2)", "short-duration power limit (PL2)"},
	"cause.vrm":              {"régulateur de tension (VRM : courant ou température)", "voltage regulator (VRM: current or temperature)"},
	"cause.other":            {"autre limite matérielle", "other hardware limit"},
	"cause.probable.thermal": {"probablement thermique (température proche de TjMax)", "probably thermal (temperature close to TjMax)"},
	"cause.probable.limit":   {"limite de performance signalée par Windows (cause exacte inconnue sans PawnIO)", "performance limit reported by Windows (exact cause unknown without PawnIO)"},
	"cause.probable.unknown": {"inconnue (niveau de mesure insuffisant)", "unknown (insufficient measurement level)"},

	"f.ram.ok":       {"%s testés, %d passe(s) complète(s), aucune erreur.", "%s tested, %d full pass(es), no errors."},
	"f.ram.errors":   {"%d erreur(s) mémoire détectée(s) (%s) : barrette défaillante, profil XMP/EXPO instable ou contrôleur mémoire en cause.", "%d memory error(s) detected (%s): faulty module, unstable XMP/EXPO profile or memory controller."},
	"f.ram.nopass":   {"Aucune passe mémoire complète : durée trop courte pour conclure.", "No complete memory pass: duration too short to conclude."},
	"f.ram.unlocked": {"Mémoire non verrouillée : une partie a pu être placée en fichier d'échange pendant le test.", "Memory not locked: part of it may have been swapped out during the test."},
	"f.ecc.ce":       {"%d erreur(s) ECC corrigée(s) pendant le test (%s) : la mémoire se dégrade.", "%d corrected ECC error(s) during the test (%s): memory is degrading."},
	"f.ecc.ue":       {"%d erreur(s) ECC NON corrigée(s) pendant le test (%s).", "%d UNCORRECTED ECC error(s) during the test (%s)."},
	"f.ecc.none":     {"Compteurs ECC non disponibles (mémoire non ECC ou pilote absent).", "ECC counters not available (non-ECC memory or missing driver)."},

	"f.disk.ok":       {"%s : %s vérifiés sans erreur ; séquentiel %.0f Mo/s en lecture, %.0f Mo/s en écriture.", "%s: %s verified without error; sequential %.0f MB/s read, %.0f MB/s write."},
	"f.disk.verify":   {"%s : %d bloc(s) relu(s) avec des données fausses : corruption silencieuse.", "%s: %d block(s) read back with wrong data: silent corruption."},
	"f.disk.io":       {"%s : %d erreur(s) d'entrée/sortie.", "%s: %d I/O error(s)."},
	"f.disk.degraded": {"%s : l'état SMART s'est dégradé pendant le test (%s).", "%s: SMART health degraded during the test (%s)."},
	"f.disk.health":   {"%s : signaux d'alerte SMART (%s).", "%s: SMART warning signs (%s)."},
	"f.disk.nohealth": {"%s : état SMART non lu (%s).", "%s: SMART health not read (%s)."},
	"f.disk.slc":      {"%s : le débit d'écriture chute de %.0f à %.0f Mo/s après ~%.0f Go (saturation du cache SLC, comportement normal).", "%s: write speed drops from %.0f to %.0f MB/s after ~%.0f GB (SLC cache saturation, normal behaviour)."},
	"f.disk.nodirect": {"%s : E/S directes indisponibles, mesures influencées par le cache.", "%s: direct I/O unavailable, measurements influenced by cache."},
	"f.disk.lat":      {"%s : latence 4K QD1 en lecture élevée (p99 %.1f ms).", "%s: high 4K QD1 read latency (p99 %.1f ms)."},

	"f.net.ok":     {"Lien vers %s : %.0f Mbit/s montant, %.0f Mbit/s descendant, pertes UDP %.2f %%.", "Link to %s: %.0f Mbit/s up, %.0f Mbit/s down, UDP loss %.2f%%."},
	"f.net.loss":   {"Pertes UDP élevées : %.2f %% (%s).", "High UDP loss: %.2f%% (%s)."},
	"f.net.reneg":  {"Le lien s'est renégocié ou coupé pendant le test (%s).", "The link renegotiated or dropped during the test (%s)."},
	"f.net.speed":  {"La vitesse du lien a changé pendant le test (%s).", "Link speed changed during the test (%s)."},
	"f.net.nicerr": {"Erreurs de la carte réseau pendant le test : %d (%s).", "Network card errors during the test: %d (%s)."},
	"f.net.errors": {"Erreurs pendant le test réseau : %s.", "Errors during the network test: %s."},

	"f.safety.thermal":  {"Arrêt d'urgence thermique : %s. Le refroidissement est insuffisant.", "Thermal emergency stop: %s. Cooling is insufficient."},
	"f.safety.battery":  {"Arrêt : %s. Branchez le secteur et relancez.", "Stopped: %s. Plug in the charger and restart."},
	"f.safety.ac":       {"Arrêt : secteur débranché. Branchez le secteur et relancez.", "Stopped: charger unplugged. Plug it in and restart."},
	"f.safety.watchdog": {"Arrêt par le watchdog : %s (gel probable).", "Stopped by the watchdog: %s (probable hang)."},
	"f.safety.user":     {"Test arrêté par l'utilisateur.", "Test stopped by the user."},
	"f.prevcrash":       {"Le test précédent (démarré le %s, phase %s) ne s'est jamais terminé : plantage, gel ou redémarrage probable.", "The previous test (started %s, phase %s) never finished: probable crash, hang or reboot."},
	"f.ref":             {"%s : %+.0f %% par rapport à la référence locale (%d mesure(s) de ce modèle).", "%s: %+.0f%% compared to the local reference (%d measurement(s) of this model)."},

	// Limites
	"lim.ram":      {"Le test mémoire n'est pas exhaustif : il porte sur des adresses virtuelles et sur la mémoire allouable seulement. Pour un test complet, utilisez un outil amorçable.", "The memory test is not exhaustive: it covers virtual addresses and allocatable memory only. For a complete test, use a bootable tool."},
	"lim.noadmin":  {"Sans droits administrateur : SMART, MSR et mode disque brut indisponibles.", "Without administrator rights: SMART, MSR and raw disk mode unavailable."},
	"lim.sensors0": {"Aucune sonde de température ou de fréquence accessible.", "No temperature or frequency sensor available."},
	"lim.sensors1": {"Niveau de mesure 1 (sans pilote) : fréquence et zones ACPI ; la cause exacte du bridage n'est pas connue. Installez PawnIO pour le niveau 2.", "Measurement level 1 (no driver): frequency and ACPI zones; the exact throttling cause is unknown. Install PawnIO for level 2."},
	"lim.sensors2": {"Niveau de mesure 2 (PawnIO) : MSR, températures par cœur, causes de bridage.", "Measurement level 2 (PawnIO): MSR, per-core temperatures, throttling causes."},
	"lim.sensors3": {"Niveau de mesure 3 (Linux) : hwmon, cpufreq et, en root sur Intel, MSR.", "Measurement level 3 (Linux): hwmon, cpufreq and, as root on Intel, MSR."},

	// Libellés du rapport
	"r.title":      {"Rapport de validation Rodage", "Rodage validation report"},
	"r.generated":  {"Généré le %s par Rodage %s", "Generated on %s by Rodage %s"},
	"r.verdict":    {"Verdict", "Verdict"},
	"r.findings":   {"Constats", "Findings"},
	"r.config":     {"Configuration détectée", "Detected configuration"},
	"r.tests":      {"Tests effectués", "Tests performed"},
	"r.sensors":    {"Sondes et niveau de confiance", "Sensors and confidence level"},
	"r.limits":     {"Limites du test", "Test limitations"},
	"r.charts":     {"Courbes", "Charts"},
	"r.machine":    {"Machine", "Machine"},
	"r.serial":     {"Numéro de série", "Serial number"},
	"r.board":      {"Carte mère", "Motherboard"},
	"r.bios":       {"BIOS", "BIOS"},
	"r.os":         {"Système", "Operating system"},
	"r.cpu":        {"Processeur", "Processor"},
	"r.threads":    {"Cœurs logiques", "Logical cores"},
	"r.simd":       {"Jeux SIMD", "SIMD sets"},
	"r.ram":        {"Mémoire", "Memory"},
	"r.admin":      {"Droits administrateur", "Administrator rights"},
	"r.yes":        {"oui", "yes"},
	"r.no":         {"non", "no"},
	"r.unknown":    {"inconnu", "unknown"},
	"r.mode":       {"Mode", "Mode"},
	"r.profile":    {"Profil", "Profile"},
	"r.duration":   {"Durée", "Duration"},
	"r.level":      {"Niveau %d", "Level %d"},
	"r.confidence": {"Confiance", "Confidence"},
	"conf.0":       {"aucune", "none"},
	"conf.1":       {"partielle", "partial"},
	"conf.2":       {"élevée", "high"},
	"conf.3":       {"élevée", "high"},
	"r.tjmax":      {"TjMax", "TjMax"},
	"r.temp":       {"Température (°C)", "Temperature (°C)"},
	"r.freq":       {"Fréquence (MHz)", "Frequency (MHz)"},
	"r.writecurve": {"Débit d'écriture séquentielle (Mo/s)", "Sequential write speed (MB/s)"},
	"r.bench":      {"Mesures", "Measurements"},
	"r.test":       {"Test", "Test"},
	"r.single":     {"1 cœur", "1 core"},
	"r.multi":      {"Tous cœurs", "All cores"},
	"r.unit":       {"Unité", "Unit"},
	"r.result":     {"Résultat", "Result"},
	"r.cores":      {"Cœurs", "Cores"},
	"r.errors":     {"Erreurs", "Errors"},
	"r.iterations": {"Itérations", "Iterations"},
	"r.passes":     {"Passes", "Passes"},
	"r.tested":     {"Testé", "Tested"},
	"r.locked":     {"Verrouillée", "Locked"},
	"r.bandwidth":  {"Débit mémoire", "Memory throughput"},
	"r.seqread":    {"Lecture séquentielle 1 Mo", "Sequential read 1 MB"},
	"r.seqwrite":   {"Écriture séquentielle 1 Mo", "Sequential write 1 MB"},
	"r.written":    {"Écrit", "Written"},
	"r.health":     {"Santé SMART (avant → après)", "SMART health (before → after)"},
	"r.phase":      {"Phase", "Phase"},
	"r.none":       {"Aucun", "None"},
	"r.compare":    {"Comparaison de rapports", "Report comparison"},
	"r.before":     {"Avant", "Before"},
	"r.after":      {"Après", "After"},
	"r.delta":      {"Écart", "Delta"},
	"r.metric":     {"Mesure", "Metric"},
	"r.mismatch":   {"Attention : les deux rapports ne portent pas sur la même machine (%s).", "Warning: the two reports are not about the same machine (%s)."},
	"r.safetyev":   {"Événements de sûreté", "Safety events"},
	"r.client":     {"côté client", "client side"},
	"r.server":     {"côté serveur", "server side"},
	"r.expected":   {"Attendu", "Expected"},
	"r.actual":     {"Lu", "Read"},
	"cli.usage": {`Rodage — testeur de charge et de stabilité matériel.

Usage : rodage-cli <commande> [options]

Commandes :
  info                     configuration détectée, sondes, disques
  bench                    mesures courtes et reproductibles
  stress                   rodage long avec verdict réussi / échoué
  server                   serveur pour le test réseau (seconde machine)
  report <rapport.json>    régénère le HTML / PDF d'un rapport
  compare <a.json> <b.json> compare deux rapports (avant / après)
  disks                    liste les disques et leur éligibilité au mode brut
  refs list|export|import  base locale de références (sans télémétrie)
  version                  version de Rodage

« rodage-cli <commande> -h » détaille les options.
Codes de sortie : 0 réussi, 1 échoué, 2 incomplet, 3 erreur d'utilisation.`, `Rodage — hardware load and stability tester.

Usage: rodage-cli <command> [options]

Commands:
  info                     detected configuration, sensors, disks
  bench                    short, reproducible measurements
  stress                   long burn-in with a pass / fail verdict
  server                   server for the network test (second machine)
  report <report.json>     re-render a report as HTML / PDF
  compare <a.json> <b.json> compare two reports (before / after)
  disks                    list disks and their raw-mode eligibility
  refs list|export|import  local reference database (no telemetry)
  version                  Rodage version

"rodage-cli <command> -h" lists the options.
Exit codes: 0 passed, 1 failed, 2 incomplete, 3 usage error.`},
	"cli.saved":      {"Rapport enregistré : %s", "Report saved: %s"},
	"cli.stopping":   {"Arrêt en cours… (Ctrl+C à nouveau pour quitter immédiatement)", "Stopping… (Ctrl+C again to quit immediately)"},
	"cli.raw.warn":   {"ATTENTION : le mode brut EFFACE TOUT le contenu de %s (%s, %s, n° de série %s).", "WARNING: raw mode ERASES ALL data on %s (%s, %s, serial %s)."},
	"cli.raw.prompt": {"Saisissez le numéro de série pour confirmer : ", "Type the serial number to confirm: "},
	"cli.raw.ok":     {"éligible au mode brut", "eligible for raw mode"},
	"cli.server":     {"Serveur Rodage prêt. Sur l'autre machine : rodage-cli stress --net <adresse de cette machine>", "Rodage server ready. On the other machine: rodage-cli stress --net <this machine's address>"},
	"cli.prevcrash":  {"Le test précédent ne s'est pas terminé (plantage ou gel probable) : ce sera signalé dans le rapport.", "The previous test did not finish (probable crash or hang): this will be noted in the report."},
	"u.mbs":          {"Mo/s", "MB/s"},
	"u.gbs":          {"Go/s", "GB/s"},
	"r.notested":     {"non testé", "not tested"},
}
