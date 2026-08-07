package nyuu

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"go-post-tools/internal/binutil"
	"go-post-tools/internal/config"
)

type Progress struct {
	Percent  float64 `json:"percent"`
	Articles string  `json:"articles"`
	Speed    string  `json:"speed"`
	ETA      string  `json:"eta"`
	Done     bool    `json:"done"`
	Error    string  `json:"error,omitempty"`
}

type Result struct {
	NZBPath string `json:"nzb_path"`
	// Warnings : non vide si nyuu a terminé en code 32 (post complet, mais des
	// erreurs ont été ignorées grâce à --skip-errors). Le NZB est exploitable,
	// mais l'utilisateur doit être prévenu.
	Warnings string `json:"warnings,omitempty"`
}

var (
	progressRegex = regexp.MustCompile(`(\d+(?:\.\d+)?)\s*%`)
	articlesRegex = regexp.MustCompile(`(\d+)\s*/\s*(\d+)`)
	speedRegex    = regexp.MustCompile(`(\d+(?:\.\d+)?)\s*([KMGT]i?B/s|B/s)`)
	etaRegex      = regexp.MustCompile(`ETA\s+(\d{2}:\d{2}(?::\d{2})?)`)
	// notableRegex : lignes qui expliquent une panne, par opposition au bruit de
	// progression. Ce sont elles qu'on remonte en tête du message d'erreur.
	// Libellés exacts de nyuu v0.4.2 : `[ERR ]`, `[WARN]` (et `[INFO]`, `[DBG ]`
	// qu'on ignore) — noter l'espace de padding dans `[ERR ]`.
	notableRegex = regexp.MustCompile(`\[(ERR|WARN|CRIT)\s*\]`)
	ansiRegex    = regexp.MustCompile("\x1b\\[[0-9;?]*[A-Za-z]")
)

// exitCodeNyuuSkipped : code retour nyuu quand le post s'est terminé mais que
// des erreurs ont été ignorées (cf. `nyuu --help-full`, section Exit codes).
const exitCodeNyuuSkipped = 32

// rawTailSize : nombre de lignes brutes conservées en secours quand nyuu meurt
// sans avoir émis la moindre ligne [ERROR]. Borné pour ne pas accumuler les
// milliers de lignes de progression d'un gros post en mémoire.
const rawTailSize = 20

func binaryPath() string {
	if path, err := binutil.ExtractBinary("nyuu"); err == nil {
		return path
	}
	if path, err := exec.LookPath("nyuu"); err == nil {
		return path
	}
	return "nyuu"
}

func Run(ctx context.Context, cfg *config.Config, inputFiles []string, nzbOutputPath string, releaseName string, onProgress func(Progress)) (*Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	args := buildArgs(cfg, inputFiles, nzbOutputPath, releaseName)
	cmd := exec.CommandContext(ctx, binaryPath(), args...)

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("pipe stderr: %w", err)
	}
	cmd.Stdout = io.Discard

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("démarrage nyuu: %w", err)
	}

	notable, rawTail := parseProgress(stderr, onProgress)

	waitErr := cmd.Wait()
	code := exitCode(waitErr)

	// Code 32 = le post est allé au bout, mais --skip-errors a laissé passer des
	// incidents (timeout, connexion perdue, check raté). Le NZB est utilisable :
	// on rend la main en succès, en remontant les incidents à l'appelant.
	if waitErr != nil && code != exitCodeNyuuSkipped {
		msg := summarize(notable, rawTail)
		onProgress(Progress{Done: true, Error: msg})
		return nil, fmt.Errorf("nyuu (code %d) : %s", code, msg)
	}

	onProgress(Progress{Percent: 100, Done: true})
	res := &Result{NZBPath: nzbOutputPath}
	if code == exitCodeNyuuSkipped {
		res.Warnings = summarize(notable, rawTail)
	}
	return res, nil
}

// exitCode extrait le code retour du process. Renvoie -1 si l'échec vient
// d'autre chose que d'une sortie non nulle (process tué, pipe cassé…).
func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode()
	}
	return -1
}

// summarize construit un message lisible : les lignes [ERROR]/[WARN] d'abord,
// puisque c'est là qu'est la cause. Sans aucune, on retombe sur les dernières
// lignes brutes plutôt que de ne rien dire.
func summarize(notable, rawTail []string) string {
	if len(notable) > 0 {
		return strings.Join(notable, "\n")
	}
	if len(rawTail) > 0 {
		return strings.Join(rawTail, "\n")
	}
	return "aucune sortie de nyuu"
}

func buildArgs(cfg *config.Config, inputFiles []string, nzbOutputPath string, releaseName string) []string {
	group := cfg.UsenetGroup
	if group == "" {
		group = "alt.binaries.test"
	}
	conns := cfg.UsenetConns
	if conns <= 0 {
		conns = 20
	}

	args := []string{
		"-h", cfg.UsenetHost,
		"-P", strconv.Itoa(cfg.UsenetPort),
		"-u", cfg.UsenetUser,
		"-p", cfg.UsenetPassword,
		"-n", strconv.Itoa(conns),
		"-g", group,
		"-o", nzbOutputPath,
		"--nzb-title", releaseName,
		"-f", "{rand(14)} {rand(14)}@{rand(5)}.{rand(3)}",
		"--message-id", "{rand(32)}@{rand(8)}.{rand(3)}",
		"--subject", "{rand(32)}",
		"--nzb-subject", `[{0filenum}/{files}] - "{filename}" yEnc ({part}/{parts})`,
		"--obfuscate-articles",
		"--overwrite",
		"--progress=stderr",
		// Par défaut nyuu s'arrête à la PREMIÈRE erreur, quelle qu'elle soit : sur
		// un post de plusieurs milliers d'articles, un aléa réseau isolé suffit à
		// tout perdre (exit 33). On absorbe les incidents non destructifs. Volon-
		// tairement PAS `post-fail` : un article réellement non posté doit rester
		// fatal, sinon on publierait un NZB troué.
		"--skip-errors", "post-timeout,check-timeout,check-missing,check-fail,connect-fail",
	}

	if cfg.UsenetSSL {
		args = append(args, "-S")
	}

	args = append(args, inputFiles...)
	return args
}

// parseProgress consomme stderr et renvoie (lignes notables, dernières lignes
// brutes). Les codes ANSI sont retirés : le scanner en laisse passer (un `\x1b[0G`
// en début de chunk finissait recollé au texte, d'où les « [0G » dans les
// messages d'erreur remontés aux utilisateurs).
func parseProgress(r io.Reader, onProgress func(Progress)) (notable, rawTail []string) {
	scanner := bufio.NewScanner(r)
	scanner.Split(scanLines)

	seen := map[string]bool{}
	for scanner.Scan() {
		line := strings.TrimSpace(ansiRegex.ReplaceAllString(scanner.Text(), ""))
		if line == "" {
			continue
		}

		if notableRegex.MatchString(line) && !seen[line] {
			seen[line] = true
			notable = append(notable, line)
		}
		rawTail = append(rawTail, line)
		if len(rawTail) > rawTailSize {
			rawTail = rawTail[1:]
		}

		p := Progress{}
		updated := false

		if m := progressRegex.FindStringSubmatch(line); len(m) >= 2 {
			if pct, err := strconv.ParseFloat(m[1], 64); err == nil {
				p.Percent = pct
				updated = true
			}
		}
		if m := articlesRegex.FindStringSubmatch(line); len(m) >= 3 {
			p.Articles = m[1] + "/" + m[2]
			updated = true
		}
		if m := speedRegex.FindStringSubmatch(line); len(m) >= 3 {
			p.Speed = m[1] + " " + m[2]
			updated = true
		}
		if m := etaRegex.FindStringSubmatch(line); len(m) >= 2 {
			p.ETA = m[1]
			updated = true
		}
		if updated {
			onProgress(p)
		}
	}
	return notable, rawTail
}

func scanLines(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}
	for i := 0; i < len(data); i++ {
		if data[i] == '\n' || data[i] == '\r' {
			return i + 1, data[:i], nil
		}
		if data[i] == 0x1b && i+3 < len(data) && data[i+1] == '[' {
			for j := i + 2; j < len(data); j++ {
				if (data[j] >= 'A' && data[j] <= 'Z') || (data[j] >= 'a' && data[j] <= 'z') {
					if i > 0 {
						return j + 1, data[:i], nil
					}
					i = j
					break
				}
			}
		}
	}
	if atEOF {
		return len(data), data, nil
	}
	return 0, nil, nil
}
