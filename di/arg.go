package di

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/samber/lo"

	"github.com/michalkurzeja/godi/v2/internal/util"
)

type Arg interface {
	fmt.Stringer
	Type() reflect.Type
}

type literalArg struct {
	v any
}

func NewLiteralArg(v any) Arg {
	return &literalArg{v: v}
}

func NewZeroArg(typ reflect.Type) Arg {
	return &literalArg{v: reflect.Zero(typ).Interface()}
}

func (a *literalArg) String() string {
	return fmt.Sprintf("%v", a.v)
}

func (a *literalArg) Type() reflect.Type {
	return reflect.TypeOf(a.v)
}

type refArg struct {
	typ reflect.Type
	def *ServiceDefinition
}

func NewRefArg(def *ServiceDefinition) (Arg, error) {
	if def == nil {
		return nil, fmt.Errorf("ref arg requires a non-nil service definition")
	}
	return &refArg{def: def}, nil
}

func (a *refArg) String() string {
	return a.def.String()
}

func (a *refArg) Type() reflect.Type {
	return a.def.Type()
}

type typeArg struct {
	typ   reflect.Type
	slice bool
}

func NewTypeArg(typ reflect.Type, slice bool) Arg {
	return &typeArg{typ: typ, slice: slice}
}

func (a *typeArg) String() string {
	return util.Signature(a.typ)
}

func (a *typeArg) Type() reflect.Type {
	if a.slice {
		return reflect.SliceOf(a.typ)
	}
	return a.typ
}

type labelArg struct {
	label Label
	typ   reflect.Type
	slice bool
}

func NewLabelArg(label Label, typ reflect.Type, slice bool) Arg {
	return &labelArg{label: label, typ: typ, slice: slice}
}

func (a *labelArg) String() string {
	return a.label.String()
}

func (a *labelArg) Type() reflect.Type {
	if a.slice {
		return reflect.SliceOf(a.typ)
	}
	return a.typ
}

type flexibleSliceArg struct {
	elemType   reflect.Type
	allowEmpty bool
}

func NewFlexibleSliceArg(elemType reflect.Type, allowEmpty bool) Arg {
	return &flexibleSliceArg{elemType: elemType, allowEmpty: allowEmpty}
}

func (a *flexibleSliceArg) String() string {
	return util.Signature(a.Type())
}

func (a *flexibleSliceArg) Type() reflect.Type {
	return reflect.SliceOf(a.elemType)
}

type compoundArg struct {
	args []Arg
	typ  reflect.Type
}

func NewCompoundArg(typ reflect.Type, args ...Arg) (Arg, error) {
	if len(args) == 0 {
		return nil, nil
	}

	for _, arg := range args {
		if !arg.Type().AssignableTo(typ) {
			return nil, fmt.Errorf("argument %s cannot be assigned to type %s", arg.Type(), typ)
		}
	}

	return &compoundArg{args: args, typ: typ}, nil
}

func (c *compoundArg) String() string {
	strs := lo.Map(c.args, func(arg Arg, _ int) string {
		return arg.String()
	})
	return strings.Join(strs, ", ")
}

func (c *compoundArg) Type() reflect.Type {
	return c.typ
}

func NewSlottedArg(arg Arg, slot uint) *SlottedArg {
	return &SlottedArg{Arg: arg, slot: slot}
}

type SlottedArg struct {
	Arg
	slot uint
}

func (a *SlottedArg) Slot() uint {
	return a.slot
}

type Slots []*Slot

func (slots Slots) Args() []Arg {
	return lo.Map(slots, func(slot *Slot, _ int) Arg {
		return slot.Arg()
	})
}

type Slot struct {
	arg      Arg
	args     []Arg
	typ      reflect.Type
	i        uint
	variadic bool
}

func NewSlot(typ reflect.Type, i uint, variadic bool) *Slot {
	return &Slot{typ: typ, i: i, variadic: variadic}
}

func (s *Slot) IsSlice() bool {
	return s.typ.Kind() == reflect.Slice
}

func (s *Slot) IsVariadicSlice() bool {
	return s.IsSlice() && s.variadic
}

func (s *Slot) Type() reflect.Type {
	return s.typ
}

func (s *Slot) ElemType() reflect.Type {
	if !s.IsSlice() {
		panic("slot is not a slice")
	}
	return s.typ.Elem()
}

func (s *Slot) FillableBy(arg Arg) bool {
	return s.SettableBy(arg) || s.AppendableBy(arg)
}

func (s *Slot) SettableBy(arg Arg) bool {
	return arg.Type().AssignableTo(s.Type())
}

func (s *Slot) AppendableBy(arg Arg) bool {
	if !s.IsSlice() {
		return false
	}
	return arg.Type().AssignableTo(s.ElemType())
}

func (s *Slot) Fill(arg Arg) error {
	if s.SettableBy(arg) {
		return s.Set(arg)
	}
	if s.AppendableBy(arg) {
		return s.Append(arg)
	}
	return fmt.Errorf("argument %s cannot fill slot %d", arg.Type(), s.i)
}

func (s *Slot) Set(arg Arg) error {
	if !s.SettableBy(arg) {
		return fmt.Errorf("argument %s cannot be assigned to slot %d", arg.Type(), s.i)
	}

	s.arg = arg
	return nil
}

func (s *Slot) Append(args ...Arg) error {
	if !s.IsSlice() {
		return fmt.Errorf("cannot add args to slot %d: slot not a slice", s.i)
	}

	for _, arg := range args {
		if !s.AppendableBy(arg) {
			return fmt.Errorf("argument %s cannot be added as slot %d element", arg.Type(), s.i)
		}
	}

	s.args = append(s.args, args...)
	return nil
}

func (s *Slot) Arg() Arg {
	if s.IsSlice() && s.arg == nil {
		a, err := NewCompoundArg(s.ElemType(), s.args...)
		if err != nil {
			panic(err) // Should never happen - we check types when adding args.
		}
		return a
	}
	return s.arg
}

func (s *Slot) IsFilled() bool {
	return s.IsSet() || s.IsAppended()
}

func (s *Slot) IsSet() bool {
	return s.arg != nil
}

func (s *Slot) IsAppended() bool {
	return len(s.args) > 0
}

func (s *Slot) Index() uint {
	return s.i
}

type ArgList struct {
	slots    Slots
	variadic bool
}

func NewArgList(fnType reflect.Type) *ArgList {
	return &ArgList{
		slots: lo.RepeatBy(fnType.NumIn(), func(i int) *Slot {
			return NewSlot(fnType.In(i), uint(i), fnType.IsVariadic() && i == fnType.NumIn()-1)
		}),
		variadic: fnType.IsVariadic(),
	}
}

func (l *ArgList) Slots() Slots {
	return l.slots
}

func (l *ArgList) Assign(arg Arg) error {
	if sArg, ok := arg.(*SlottedArg); ok {
		return l.FillSlot(sArg)
	}

	for _, slot := range l.slots {
		if slot.IsSet() || !slot.FillableBy(arg) {
			continue
		}
		return slot.Fill(arg)
	}

	return fmt.Errorf("argument %s cannot be slotted to function", arg.Type())
}

func (l *ArgList) ValidateAndCollect() ([]Arg, error) {
	if err := l.Validate(); err != nil {
		return nil, err
	}

	slotsCount := len(l.slots)
	args := make([]Arg, slotsCount)
	copy(args, l.slots.Args())

	if l.IsVariadic() && args[slotsCount-1] == nil {
		args[slotsCount-1] = NewZeroArg(l.slots[slotsCount-1].Type())
	}

	return args, nil
}

func (l *ArgList) Validate() error {
	requiredCount := len(l.slots)
	if l.IsVariadic() {
		requiredCount--
	}

	unfilledCount := lo.CountBy(l.slots, func(slot *Slot) bool { return !slot.IsVariadicSlice() && !slot.IsFilled() })
	if unfilledCount == 0 {
		return nil
	}

	filledCount := requiredCount - unfilledCount
	if l.IsVariadic() {
		return fmt.Errorf("function requires at least %d arguments, got %d", requiredCount, filledCount)
	}
	return fmt.Errorf("function requires %d arguments, got %d", requiredCount, filledCount)
}

func (l *ArgList) IsVariadic() bool {
	return l.variadic
}

func (l *ArgList) FillSlot(arg *SlottedArg) error {
	if slotsCount := uint(len(l.slots)); arg.Slot() >= slotsCount {
		return fmt.Errorf("argument %s is assigned to slot %d, but function has only %d argument slots", util.Signature(arg.Type()), arg.Slot(), slotsCount)
	}
	return l.slots[arg.Slot()].Fill(arg.Arg)
}

func (l *ArgList) SetSlot(arg *SlottedArg) error {
	if slotsCount := uint(len(l.slots)); arg.Slot() >= slotsCount {
		return fmt.Errorf("argument %s is assigned to slot %d, but function has only %d argument slots", util.Signature(arg.Type()), arg.Slot(), slotsCount)
	}
	return l.slots[arg.Slot()].Set(arg.Arg)
}

func (l *ArgList) AppendSlot(arg *SlottedArg) error {
	if slotsCount := uint(len(l.slots)); arg.Slot() >= slotsCount {
		return fmt.Errorf("argument %s is assigned to slot %d, but function has only %d argument slots", util.Signature(arg.Type()), arg.Slot(), slotsCount)
	}
	return l.slots[arg.Slot()].Append(arg.Arg)
}
