package core

// Relation cardinality.
const (
	CardinalityOne  = "one"  // ForeignKey / OneToOne: a single related object (or nil)
	CardinalityMany = "many" // ManyToMany: a collection of related objects
)

// Relation is a ForeignKey/OneToOne/ManyToMany field's link to another
// ModelAdmin. Target is a slug looked up on the registry at render
// time -- never a direct reference, so relations don't force
// import-order coupling between ModelAdmins.
type Relation struct {
	Name         string
	Target       string
	DisplayField string
	Cardinality  string
	GetRelated   func(obj any) any
}

func (r Relation) GetValue(obj any) any {
	if r.GetRelated != nil {
		return r.GetRelated(obj)
	}
	v, _ := structFieldOrMapValue(obj, r.Name)
	return v
}
