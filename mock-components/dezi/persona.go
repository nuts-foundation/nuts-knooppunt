package main

// Practitioner is the synthetic healthcare professional this mock attests to.
// Field names follow the Dezi v0.7 subject rather than the Dutch claim names.
type Practitioner struct {
	DeziNumber    string
	Initials      string
	SurnamePrefix string
	Surname       string
	OrgURA        string
	OrgName       string
	RoleCode      string
	RoleName      string
	RoleRegistry  string
}

// DrElAmrani is the canonical requesting user of the GF Sandbox demo
// (sandbox/DESIGN.md section 5.2). RoleCode is a RoleCodeNL value from the
// Nictiz value set for OID 2.16.840.1.113883.2.4.15.111; 01.022 is the
// hospital geriatrician role, matching De Plataan as the consuming hospital.
var DrElAmrani = Practitioner{
	DeziNumber:   "900001234",
	Initials:     "S.",
	Surname:      "el Amrani",
	OrgURA:       "00000010",
	OrgName:      "Ziekenhuis De Plataan",
	RoleCode:     "01.022",
	RoleName:     "Klinisch geriater",
	RoleRegistry: "http://www.dezi.nl/rol_bron/big",
}
