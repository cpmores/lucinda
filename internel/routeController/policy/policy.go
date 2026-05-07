package policy

type PolicyType string

const (
	PolicyTypeDivide PolicyType = "divide"
	PolicyTypeRoute  PolicyType = "route"
	PolicyTypePost   PolicyType = "post"
	PolicyTypeReduce PolicyType = "reduce"

	PolicyTypeCapable PolicyType = "capable"
)

type Policy struct {
	ID      string
	Version string
	Type    PolicyType
	Desc    string
}

func (p Policy) GetPolicyKey() string {
	return p.ID + "-" + p.Version
}

