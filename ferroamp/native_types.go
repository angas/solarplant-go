package ferroamp

import (
	"github.com/angas/solarplant-go/calc"
)

type ValueObject[T ~float64 | ~int64] struct {
	Value T `json:"val,string"`
}

type FltObj ValueObject[float64]
type IntObj ValueObject[int64]

type StrObj struct {
	Value string `json:"val"`
}

type Phases struct {
	L1 float64 `json:"l1,string"`
	L2 float64 `json:"l2,string"`
	L3 float64 `json:"l3,string"`
}

type EhubMessage struct {
	GridFreq      FltObj  `json:"gridfreq"`         // Estimated Grid Frequency(Hz)
	Ul            Phases  `json:"ul"`               // External voltage (V)
	Iace          Phases  `json:"iace"`             // ACE equalization current set-points in Arms (A)
	Il            Phases  `json:"il"`               // Inverter RMS current (A)
	Ild           Phases  `json:"ild"`              // Inverter reactive current (A)
	Ilq           Phases  `json:"ilq"`              // Inverter active current (A)
	Iext          Phases  `json:"iext"`             // External/grid RMS current (A)
	Iextd         Phases  `json:"iextd"`            // External/grid reactive current (A)
	Iextq         Phases  `json:"iextq"`            // External/grid active current (A)
	ILoadd        *Phases `json:"iLoadd,omitempty"` // (A)
	ILoadq        *Phases `json:"iLoadq,omitempty"` // (A)
	Soc           FltObj  `json:"soc"`              // State Of Charge for the system (%)
	Soh           FltObj  `json:"soh"`              // State Of Health for the system (%)
	Sext          FltObj  `json:"sext"`             // Apparent power (VA)
	Pext          Phases  `json:"pext"`             // External/grid power, active (W)
	PextReactive  Phases  `json:"pextreactive"`     // External/grid power, reactive (W)
	Pinv          Phases  `json:"pinv"`             // Inverter power active (W)
	PinvReactive  Phases  `json:"pinvreactive"`     // Inverter power active (W)
	Pload         Phases  `json:"pload"`            // (W)
	PloadReactive Phases  `json:"ploadreactive"`    // (W)
	Ppv           FltObj  `json:"ppv"`              // Only sent when system has PV (W)
	Pbat          FltObj  `json:"pbat"`             // Only sent when system has batteries (W)
	RatedCap      FltObj  `json:"ratedcap"`         // Total rated capacity of all batteries in system (Wh)
	WextProdQ     Phases  `json:"wextprodq"`        // (mJ) Claude: Represents energy produced and exported to the external grid
	WextConsQ     Phases  `json:"wextconsq"`        // (mJ) Claude: Represents energy consumed/imported from the external grid
	WinvProdQ     Phases  `json:"winvprodq"`        // (mJ)
	WinvConsQ     Phases  `json:"winvconsq"`        // (mJ)
	WloadProdQ    Phases  `json:"wloadprodq"`       // (mJ)
	WloadConsQ    Phases  `json:"wloadconsq"`       // (mJ)
	Wpv           FltObj  `json:"wpv"`              // Only sent when system has PV (mJ)
	WbatProd      FltObj  `json:"wbatprod"`         // Only sent when system has batteries (mJ)
	WbatCons      FltObj  `json:"wbatcons"`         // Only sent when system has batteries (mJ)
	State         FltObj  `json:"state"`            // State of the system
	Udc           Udc     `json:"udc"`              // Positive and negative DC Link voltage (V)
	Ts            StrObj  `json:"ts"`               // Time stamp when message was published
}

type Udc struct {
	Neg float64 `json:"neg,string"`
	Pos float64 `json:"pos,string"`
}

type SsoMessage struct {
	ID          StrObj `json:"id"`          // Unique identifier of SSO
	Upv         FltObj `json:"upv"`         // Voltage measured on PV string side (V)
	Ipv         FltObj `json:"ipv"`         // Current measured on PV string side (A)
	Wpv         IntObj `json:"wpv"`         // Total energy produced by SSO (mJ)
	FaultCode   IntObj `json:"faultcode"`   // 0x00 = OK For all other values please contact Ferroamp support
	RelayStatus IntObj `json:"relaystatus"` // 0 = relay closed (i.e running power), 1 = relay open/disconnected, 2 = precharge
	Temp        FltObj `json:"temp"`        // Temperature measured on PCB of SSO (°C)
	Udc         FltObj `json:"udc"`         // DC link voltage as measured by SSO (V)
	Ts          StrObj `json:"ts"`          // Time stamp when message was published
}

type EsoMessage struct {
	ID          StrObj `json:"id"`          // Unique identifier
	Ubat        FltObj `json:"ubat"`        // Voltage measured on battery side (V)
	Ibat        FltObj `json:"ibat"`        // Current measured on battery side (A)
	WbatProd    IntObj `json:"wbatprod"`    // Total energy produced by ESO, i.e., total energy discharged (mJ)
	WbatCons    IntObj `json:"wbatcons"`    // Total energy consumed by ESO, i.e., total energy charged (mJ)
	Soc         FltObj `json:"soc"`         // State of Charge for ESO (0-100%)
	RelayStatus IntObj `json:"relaystatus"` // 0 = relay closed, 1 = relay open
	Temp        FltObj `json:"temp"`        // Temperature measured inside ESO (°C)
	FaultCode   IntObj `json:"faultcode"`   // Detailed fault codes
	Udc         FltObj `json:"udc"`         // DC link voltage as measured by ESO (V)
	Ts          StrObj `json:"ts"`          // Timestamp when message was published
}

type EsmMessage struct {
	ID            StrObj `json:"id"`            // Unique identifier
	Soh           FltObj `json:"soh"`           // State of Health for the system (%)
	Soc           FltObj `json:"soc"`           // State of Charge (%)
	RatedCapacity FltObj `json:"ratedCapacity"` // Rated capacity of battery (Wh)
	RatedPower    FltObj `json:"ratedPower"`    // Rated power of battery (W)
	Status        IntObj `json:"status"`        // Dependent on battery manufacturer (bitmask)
	Ts            StrObj `json:"ts"`            // Timestamp when message was published
}

type ControlResponseMessage struct {
	TransId string `json:"transId"`
	Status  string `json:"status"` // "ack" or "nack"
	Message string `json:"msg"`    // Message
}

type ControlEventMessage struct {
	Timestamp string `json:"timestamp"`
	Event     string `json:"event"`
}

var esoFaultsCodes = map[uint16]string{
	0x0001: "The pre-charge from battery to ESO is not reaching the voltage goal prohibiting the closing of the relays.",
	0x0002: "CAN communication issues between ESO and battery.",
	0x0004: "This indicates that the SoC limits for the batteries are not configured correctly, please contact Ferroamp Support for help.",
	0x0008: "This indicates that the power limits for the batteries are incorrect or non-optimal. When controlling batteries via extapi and the system is set in either peak-shaving or self-consumption modes this flag may be set but it will not affect control. When not controlling batteries via extapi this indicates that the settings made in EMS Configuration is invalid.",
	0x0010: "On-site emergency stop has been triggered.",
	0x0020: "The DC-link voltage in ESO is so high that it prevents operation.",
	0x0040: "Indicates that the battery has an alarm or an error flag raised. Please check Battery manufacturer's manual for trouble shooting the battery, or call Ferroamp Support.",
	0x0080: "Not a fault, just an indication that Battery Manufacturer is not Ferroamp.",
	0x0100: "Not used",
	0x0200: "Not used",
	0x0400: "Not used",
	0x0800: "Not used",
	0x1000: "Not used",
	0x2000: "Not used",
	0x4000: "Not used",
}

// WARNING: Don't use this, values aren't updated as they should
func (ehub *EhubMessage) LifetimeProduced() float64 {
	return calc.MJ2Kwh(ehub.WloadProdQ.L1 + ehub.WloadProdQ.L2 + ehub.WloadProdQ.L3)
}

func (ehub *EhubMessage) LifetimeConsumed() float64 {
	return calc.MJ2Kwh(ehub.WloadConsQ.L1 + ehub.WloadConsQ.L2 + ehub.WloadConsQ.L3)
}
