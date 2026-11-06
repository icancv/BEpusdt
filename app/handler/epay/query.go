package epay

import (
	"crypto/subtle"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/spf13/cast"
	"github.com/v03413/bepusdt/app/model"
)

// Api 【兼容】易支付 api.php，目前仅支持 act=order（查单）
//
// 上游客户端（如 sub2api）会同时通过 URL query 和 form body 传参，这里两处都读取。
// 只有 OrderStatusSuccess 才返回 status=1，确认中等中间状态一律视为未支付，避免下游提前入账。
func (e Epay) Api(ctx *gin.Context) {
	if err := ctx.Request.ParseForm(); err != nil {
		e.queryFail(ctx, "请求参数解析失败："+err.Error())

		return
	}

	var param = func(key string) string {

		return ctx.Request.Form.Get(key)
	}

	if param("act") != "order" {
		e.queryFail(ctx, "不支持的 act，目前仅支持 order")

		return
	}

	if param("pid") != Pid {
		e.queryFail(ctx, "商户号【PID】必须固定为"+Pid)

		return
	}

	// 令牌为空时拒绝所有请求，避免空 key 与空令牌比较相等
	if token := model.AuthToken(); token == "" || subtle.ConstantTimeCompare([]byte(param("key")), []byte(token)) != 1 {
		e.queryFail(ctx, "商户密钥错误")

		return
	}

	var outTradeNo, tradeNo = param("out_trade_no"), param("trade_no")
	if outTradeNo == "" && tradeNo == "" {
		e.queryFail(ctx, "参数 out_trade_no 或 trade_no 必须提供一个")

		return
	}

	var order model.Order
	var found bool
	if tradeNo != "" {
		order, found = model.GetTradeOrder(tradeNo)
		// 同时提供两个参数时必须指向同一笔订单
		if found && outTradeNo != "" && order.OrderId != outTradeNo {
			found = false
		}
	} else {
		// 与 StartBuildOrder 一致：同一商户订单号取最新一条
		res := model.Db.Where("order_id = ?", outTradeNo).Order("id desc").Limit(1).Find(&order)
		found = res.RowsAffected > 0
	}

	if !found {
		e.queryFail(ctx, "订单不存在")

		return
	}

	var status = 0
	if order.Status == model.OrderStatusSuccess {
		status = 1
	}

	ctx.JSON(http.StatusOK, gin.H{
		"code":         1,
		"msg":          "succ",
		"trade_no":     order.TradeId,
		"out_trade_no": order.OrderId,
		"type":         order.TradeType,
		"pid":          Pid,
		"name":         order.Name,
		"money":        cast.ToString(order.Money), // 与订单回调中的 money 保持一致
		"status":       status,
	})
}

func (e Epay) queryFail(ctx *gin.Context, msg string) {
	ctx.JSON(http.StatusOK, gin.H{"code": -1, "msg": msg})
}
