# Go 钃濆浘寮曟搸 Agent 瑙勫垯

鏈洰褰曞寘鍚潰鍚戞湇鍔″櫒杩愯鐨?Go 钃濆浘瑙ｆ瀽涓庢墽琛屽紩鎿庛€傝繖閲屽睘浜庨珮椋庨櫓杩愯鏃朵唬鐮侊紝淇敼鏃跺繀椤讳紭鍏堣€冭檻鍏煎鎬с€佺嚎绋嬪畨鍏ㄥ拰鎬ц兘銆?
## 蹇呰涓婁笅鏂?
淇敼鏈洰褰曞墠锛屽厛闃呰锛?
- `../../docs/CODEX_BLUEPRINT_ENGINE_RULES_ZH.md`
- `../../docs/BLUEPRINT_ENGINE_TEST_MATRIX_ZH.md`
- 濡傛灉娑夊強 legacy `.vgf` 琛屼负锛岃繕瑕侀槄璇?`../../docs/LEGACY_COMPATIBILITY_ZH.md`

## 纭€ц鍒?
- 涓嶈鎶婂崟娆℃墽琛岀殑鍙彉鐘舵€佹斁鍒?`CompiledGraph`銆乣ExecNode` 鎴?`NodeDefinition` 涓婏紱瀹冧滑鏄叡浜彧璇昏繍琛屾椂缁撴瀯銆?- `Graph` 鏄崟娆℃墽琛?session锛屼笉鑳藉苟鍙戝鐢ㄣ€?- 鏈嶅姟鍣ㄤ唬鐮佸簲閫氳繃 `Blueprint` 璋冪敤锛沗Blueprint` 鏄澶栧苟鍙戝畨鍏?facade銆?- 寮傛 continuation 鐘舵€佸繀椤诲彧灞炰簬琚寕璧风殑 `Graph` session銆?- 濡傛灉鑺傜偣鍙兘 suspend锛屽湪 continuation 瀹屾垚鍓嶏紝涓嶈兘鎶婄浉鍏虫墽琛屽璞″綊杩樻睜鎴栧鐢ㄣ€?- 淇濇寔 `.vgf` 鍏煎鎬с€傚凡鍒犻櫎鎴栨湭鐭ョ殑 legacy 鑺傜偣搴旈殣钘忔垨淇濈暀锛屼笉鑳介潤榛樹涪寮冦€?- 椤跺眰 `nodes/*.json` 鏄郴缁熻妭鐐瑰畾涔夈€傞櫎闈炵敤鎴锋槑纭姹傦紝`nodes/json/**` 涓氬姟瀹氫箟涓嶅湪澶勭悊鑼冨洿銆?- 鏂囦欢銆佽〃鏍笺€佸瓧鍏歌摑鍥炬暟鎹被鍨嬪凡鎸夐渶姹傚垹闄ゃ€傛湭缁忕敤鎴锋槑纭悓鎰忥紝涓嶈鎭㈠銆?
## 鎬ц兘瑙勫垯

- 浼樺厛鍋氱紪璇戞湡鎴栧姞杞芥湡棰勫鐞嗭紝鑰屼笉鏄墽琛屾湡鏌ユ壘銆?- `CompileGraph` 杩斿洖鍚庯紝缂栬瘧缁撴瀯蹇呴』淇濇寔涓嶅彲鍙樸€?- 鐑墽琛岃矾寰勪笂锛屽鏋滆兘浣跨敤 index 鎴栭璁＄畻 binding锛屽氨涓嶈浣跨敤 string-keyed map銆?- 涓嶈鍦ㄥ叡浜妭鐐逛笂缂撳瓨 `ExecContext` 鎴?port 鍊笺€?- 瀵硅薄姹犲繀椤昏€冭檻寮傛 continuation 鐢熷懡鍛ㄦ湡锛涜鎸傝捣鐨勭姸鎬佺粷涓嶈兘褰掕繕姹犮€?
## 楠岃瘉鍛戒护

淇敼 engine 鏃讹紝鍏堣窇绐勬祴璇曪紝鐒跺悗鑷冲皯杩愯锛?
```powershell
go test ./engine/go/blueprint -count=1
go test -race ./engine/go/blueprint -count=1
```

淇敼 facade 鎴栫嚎绋嬪畨鍏ㄧ浉鍏充唬鐮佹椂锛岃繕瑕佽繍琛岋細

```powershell
go test -race ./... -count=1
```

淇敼鎬ц兘鏁忔劅璺緞鏃讹紝杩愯锛?
```powershell
go test ./engine/go/blueprint -run '^$' -bench 'BenchmarkBlueprintDo(Shared|Complex)|BenchmarkFunctionCall' -benchtime=3s -benchmem -count=1
```
