# Cisco-like Ping for Go

Cisco IOS 風の ping 出力を再現する Go 実装。
`!` = echo reply, `U` = destination unreachable, `.` = timeout / その他。

## Build

```sh
go build -o cping
```

## Usage

```sh
sudo ./cping ping.kooshin.net
sudo ./cping -c 1400 ping.kooshin.net   # ASCII アート向け
sudo ./cping ping.kooshin.net -c 0      # 無限 (Ctrl-C で停止) — フラグは <dst> の前後どちらでも可
```

raw ICMP socket を使うため `sudo` 推奨（または `CAP_NET_RAW`）。

### Flags

| Flag | Default | Meaning |
| :--- | :--- | :--- |
| `-c`, `--count N` | `5` | 送信回数（`0` = 無限） |
| `-s`, `--size N` | `56` | ICMP payload サイズ (bytes) |
| `-W`, `--timeout S` | `2` | パケット毎のタイムアウト (秒, 小数可) |
| `-l`, `--width N` | `70` | 改行幅（1行の記号数） |
| `-v`, `--verbose` | | 詳細ログを stderr に |
| `-q`, `--quiet` | | 記号出力を抑制（統計のみ） |

`--ttl / --df / --source / --interval / --tos / --pattern / --ipv6 / --pace / --linger` は受理するが現状未実装 (NYI)。

## Sample output

```
$ sudo ./cping ping.kooshin.net
!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!
!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!
!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!
!!UU!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!UU!!!!!!!!
Success rate is 99 percent (1394/1400), round-trip min/avg/max = 12/18/45 ms
```

## History

このリポジトリはもともと Python + scapy で書かれた `cping-py` でした。
2026-04-30 に Go へ書き換え、リポジトリ名を `cping-go` に変更しています。
旧 Python 実装は git 履歴に保存されています。
