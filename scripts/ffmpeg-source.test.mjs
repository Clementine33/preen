import assert from 'node:assert/strict'
import { spawnSync } from 'node:child_process'
import { mkdtempSync, readFileSync, rmSync } from 'node:fs'
import { tmpdir } from 'node:os'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { test } from 'node:test'

const root = fileURLToPath(new URL('../', import.meta.url))

const harness = `
sed() { shift; command sed -n "$1" "$SOURCE_TEST_ROOT/mise.toml"; }
curl() {
  printf '%s\\n' "$*" >> "$SOURCE_TEST_LOG"
  case " $* " in *' --http1.1 '*) ;; *) return 35 ;; esac
  case " $* " in *' --retry-all-errors '*) ;; *) return 35 ;; esac
  case " $* " in *' --connect-timeout 15 '*) ;; *) return 28 ;; esac
  while [ "$#" -gt 0 ]; do
    case "$1" in
      -o) shift; output=$1 ;;
      https://*) url=$1 ;;
    esac
    shift
  done
  printf '%s' partial > "$output"
  case "$url" in https://ffmpeg.org/*) return 35 ;; esac
  if [ "$SOURCE_TEST_MODE" = unavailable ]; then return 28; fi
  printf '%s' complete > "$output"
}
gpg() {
  case " $* " in
    *' --verify '*)
      if [ "$SOURCE_TEST_MODE" = bad-signature ]; then return 1; fi
      if [ "$SOURCE_TEST_MODE" = wrong-signer ]; then
        echo '[GNUPG:] VALIDSIG 0000000000000000000000000000000000000000'
      else
        echo '[GNUPG:] VALIDSIG subkey 0 0 0 0 0 0 0 0 FCF986EA15E6E293A5644F10B4322F04D67658D8'
      fi ;;
  esac
}
tar() {
  test "$(cat ffmpeg.tar.xz)" = complete || return 1
  echo extracted >> "$SOURCE_TEST_LOG"
}
export -f sed curl gpg tar
cd "$SOURCE_TEST_DIR"
bash "$SOURCE_TEST_ROOT/scripts/ffmpeg-source.sh"
`

for (const mode of ['fallback', 'unavailable', 'bad-signature', 'wrong-signer']) {
  test(`FFmpeg source download: ${mode}`, { skip: process.platform === 'win32' }, (t) => {
    const dir = mkdtempSync(path.join(tmpdir(), 'preen-source-'))
    t.after(() => rmSync(dir, { recursive: true, force: true }))
    const log = path.join(dir, 'calls')
    const result = spawnSync('bash', ['-c', harness], {
      cwd: root,
      encoding: 'utf8',
      env: { ...process.env, SOURCE_TEST_ROOT: root, SOURCE_TEST_DIR: dir, SOURCE_TEST_LOG: log, SOURCE_TEST_MODE: mode, TMPDIR: dir },
    })
    assert.ifError(result.error)
    assert.equal(result.status === 0, mode === 'fallback', result.stderr)
    const calls = readFileSync(log, 'utf8')
    assert.equal(calls.includes('extracted'), mode === 'fallback')
    if (mode === 'fallback') {
      assert.match(calls, /https:\/\/distfiles\.gentoo\.org\/distfiles\/[0-9a-f]{2}\/ffmpeg-[\d.]+\.tar\.xz /)
      assert.match(calls, /https:\/\/distfiles\.gentoo\.org\/distfiles\/[0-9a-f]{2}\/ffmpeg-[\d.]+\.tar\.xz\.asc /)
      assert.match(calls, /https:\/\/raw\.githubusercontent\.com\/FFmpeg\/web\/master\/htdocs\/ffmpeg-devel\.asc /)
    }
  })
}
