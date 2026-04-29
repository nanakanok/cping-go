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
```

raw socket を使わない場合（Linux で `net.ipv4.ping_group_range` が許可されていれば動く）:

```sh
./cping -privileged=false ping.kooshin.net
```

### Flags

| Flag | Default | Meaning |
| :--- | :--- | :--- |
| `-c` | `1400` | 送信パケット数 |
| `-s` | `56` | payload サイズ (bytes) |
| `-W` | `2s` | パケット毎のタイムアウト |
| `-i` | `0` | 送信間隔 (0 = 連続送信) |
| `-privileged` | `true` | raw ICMP socket を使う (要 root / CAP_NET_RAW) |

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
