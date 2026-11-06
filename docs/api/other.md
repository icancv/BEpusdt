# 第三方程序对接指南

## 兼容性说明

关于第三方程序的适配对接问题，建议您优先向您的程序开发者反馈，或在相关技术社区寻求支持，具体的适配进度和时效性无法保证。

### 兼容性特性

1. **Epusdt 兼容**：BEpusdt 与 Epusdt 插件保持高度兼容，理论上所有已适配 `Epusdt` 的程序均可无缝迁移至 `BEpusdt`
   。建议在正式环境部署前，先在测试环境中进行充分验证。

2. **彩虹易支付兼容**：原生支持`彩虹易支付`的 `submit.php` 提交接口标准，因此大多数已适配`彩虹易支付`的程序可直接集成使用。

### 易支付查单接口

除 `submit.php` 外，还兼容易支付的 `api.php?act=order` 查单接口。部分程序（如 sub2api）在收到回调后会回查订单状态，确认已支付才会入账。

- 地址：`/api.php`，GET / POST 均可，参数可放在 URL 或表单中
- 参数：

| 参数 | 必填 | 说明 |
|---|---|---|
| act | 是 | 固定为 `order` |
| pid | 是 | 固定为 `1000` |
| key | 是 | 对接令牌（与签名使用的 API Token 相同） |
| out_trade_no | 二选一 | 商户订单号，同一订单号存在多条记录时返回最新一条 |
| trade_no | 二选一 | BEpusdt 订单号；与 `out_trade_no` 同时提供时必须指向同一订单 |

- 响应示例：

```json
{"code":1,"msg":"succ","trade_no":"...","out_trade_no":"...","type":"usdt.trc20","pid":"1000","name":"...","money":"10.5","status":1}
```

`status` 为 `1` 仅表示订单已交易确认成功；等待支付、确认中、过期、取消、失败均为 `0`。`money` 与订单回调中的 `money` 一致。出错时返回 `{"code":-1,"msg":"..."}`。

注意：`key` 位于 URL 时，请避免在反向代理或网关的访问日志中记录 query。BEpusdt 自身的访问日志已不记录 query。

## 对接教程

以下是常见程序的对接文档，请根据您的实际需求选择相应教程进行配置。如遇问题，请仔细核对操作步骤，或在相关技术社区寻求协助。

- [独角Dujiao-Next](./dujiao-next/dujiao-next.md)
- [独角数卡(旧版)](./dujiaoka/dujiaoka.md)
- [彩虹易支付](https://github.com/v03413/Epay-BEpusdt)
- [WHMCS](https://github.com/v03413/whmcs-gateway-epusdt)
- [异次元发卡](acg-faka/README.md)
- [萌次元商城](./mcy-faka/mcy-shop.md)
- [EdgeKey](./edge-key/edge-key.md)
- [WooCommerce/WordPress 收款插件-BEpusdt for WooCommerce](https://github.com/immonsterx/bepusdt-woocommerce)
