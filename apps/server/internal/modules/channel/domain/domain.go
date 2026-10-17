// Package domain channel 领域规则：状态机（#16）。
package domain

import "catcube/internal/kernel/statemachine"

// 运营商状态（与 contract.Status* 对齐）。
const (
	StateApplying  = "0" // 申请中
	StateOpened    = "1" // 已开通
	StateSuspended = "2" // 已停用
)

// MachineOperator 运营商状态机：#16 申请中→已开通→已停用（停用须重新申请）。
var MachineOperator = statemachine.Def{
	Name:   "channel.operator",
	States: []string{StateApplying, StateOpened, StateSuspended},
	Transitions: []statemachine.Transition{
		{From: StateApplying, Event: "open", To: StateOpened, Actor: "*"},
		{From: StateOpened, Event: "suspend", To: StateSuspended, Actor: "*"},
		{From: StateSuspended, Event: "reapply", To: StateApplying, Actor: "*"},
	},
}
