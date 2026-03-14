# goccstatusline

A claude code statusline as a static binay beacuse npx is scary.  
Output inspired by [https://github.com/strayer/dotfiles](https://github.com/strayer/dotfiles/blob/cc8b3f259cbf7e259971d04195be53185ee874b1/dot_claude/executable_statusline.sh)

## Installation

Download the latest release from the [releases page](https://github.com/schmoaaaaah/goccstatusline/releases).  
Verify the [checksum](https://github.com/schmoaaaaah/goccstatusline/releases/latest/download/checksums.txt) and attestation (`gh attestation verify`) if you want to be extra sure.  
Make it executable and move it to your PATH.  
Add it to your ClaudeCode configuration (`~/.claude/settings.json`):

```json
{
  "statusLine": {
    "type": "command",
    "command": "/path/to/goccstatusline",
    "padding": 2
  }
}
```

## Time Comparison

a crude comparison of the execution time of the bash script, ccstatusline and the go binary on the same input.

### BASH

[https://github.com/strayer/dotfiles](https://github.com/strayer/dotfiles/blob/cc8b3f259cbf7e259971d04195be53185ee874b1/dot_claude/executable_statusline.sh)

```bash
time ./executable_statusline.sh < test/full.json
[Opus]  | +156/-23
[■■■■■■■■■■■■■□□□□□□□□□□□□□□□□□] 45% | 110k free | 1h1m | $1.23

________________________________________________________
Executed in   22.14 millis    fish           external
   usr time   12.27 millis    0.00 micros   12.27 millis
   sys time   12.98 millis  429.00 micros   12.55 millis
```

### NPX

[ccstatusline](https://github.com/sirmalloc/ccstatusline)

```bash
time npx -y ccstatusline@latest < test/full.json
Model: Claude Opus | Ctx: 15.5k | ⎇ no git | (no git)

________________________________________________________
Executed in  388.62 millis    fish           external
   usr time  314.65 millis  465.00 micros  314.19 millis
   sys time   69.75 millis   94.00 micros   69.66 millis
```

### GO

```bash
time ./goccstatusline < test/full.json
[Opus]  | +156/-23
[■■■■■■■■■■■■■□□□□□□□□□□□□□□□□□] 45% | 110k free | 1h1m | $1.23

________________________________________________________
Executed in    2.85 millis    fish           external
   usr time    1.73 millis  411.00 micros    1.32 millis
   sys time    1.31 millis    0.00 micros    1.31 millis
```
