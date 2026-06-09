package network

import (
	utils "github.com/Grape-Chain/Grape-Dag/utils"
	"github.com/enescakir/emoji"
)

func portMappingAdded(pm *PortMapping) {
	utils.ColorizePrint("\n%s%s : %s\n", emoji.Laptop, emoji.ElectricPlug, pm.String())
}
