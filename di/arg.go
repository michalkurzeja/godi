package di

import (
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/samber/lo"

	"github.com/michalkurzeja/godi/v2/internal/errorsx"
	"github.com/michalkurzeja/godi/v2/internal/util"
)

// Arg is one argument of a factory, a method call or a function: what to pass,
// and how to find it. Each kind of argument validates and resolves itself.
//
// Its methods are unexported, so only godi can add a kind. The compiler passes
// and the dependency graph have to understand every one of them.
type Arg interface {
	fmt.Stringer
	Type() reflect.Type

	// validate reports whether this argument can be resolved in the scope.
	validate(scope *Scope) error
	// resolve produces the value to pass.
	resolve(scope *Scope) (any, error)
	// resolveIDs lists the definitions this argument resolves to. A literal
	// names no service, so its list is empty.
	resolveIDs(scope *Scope) []ID
	// trace says how the argument resolved: what matched, and by which
	// mechanism. See ArgTrace.
	trace(scope *Scope, path tracePath) ArgTrace
}

// ValidateArg reports whether the argument can be resolved in the scope.
func ValidateArg(scope *Scope, arg Arg) error {
	if arg == nil {
		return fmt.Errorf("unsupported arg type %T", arg)
	}
	return arg.validate(scope)
}

// ResolveArg produces the value the argument stands for.
func ResolveArg(scope *Scope, arg Arg) (any, error) {
	if arg == nil {
		return reflect.Value{}, fmt.Errorf("unsupported arg type %T", arg)
	}
	return arg.resolve(scope)
}

// ResolveArgIDs lists the definitions the argument resolves to. A literal names
// no service, so its list is empty.
func ResolveArgIDs(scope *Scope, arg Arg) []ID {
	if arg == nil {
		return nil
	}
	return arg.resolveIDs(scope)
}

// TraceArg describes how the argument resolves in the scope.
func TraceArg(scope *Scope, arg Arg) ArgTrace {
	if arg == nil {
		return ArgTrace{}
	}
	return arg.trace(scope, tracePath{})
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

func (a *literalArg) validate(_ *Scope) error {
	return nil
}

func (a *literalArg) resolve(_ *Scope) (any, error) {
	return a.v, nil
}

func (a *literalArg) resolveIDs(_ *Scope) []ID {
	return nil
}

func (a *literalArg) trace(_ *Scope, path tracePath) ArgTrace {
	return ArgTrace{Kind: ArgKindLiteral, Value: a.v, Bindings: path.hops}
}

type refArg struct {
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

func (a *refArg) validate(scope *Scope) error {
	if !scope.HasServiceInChain(a.def.ID()) {
		return fmt.Errorf("service %s not found", a.def.ID())
	}
	return nil
}

func (a *refArg) resolve(scope *Scope) (any, error) {
	v, err := scope.GetServiceInChain(a.def.ID())
	if err != nil {
		return nil, errorsx.Wrap(err, "failed to resolve ID arg")
	}
	if v == nil {
		return nil, fmt.Errorf("service %s not found", a.def.ID())
	}
	return v, nil
}

func (a *refArg) resolveIDs(_ *Scope) []ID {
	return []ID{a.def.ID()}
}

func (a *refArg) trace(_ *Scope, path tracePath) ArgTrace {
	return ArgTrace{
		Kind:     ArgKindRef,
		Bindings: path.hops,
		Matches:  []ID{a.def.ID()},
		By:       ResolutionRef,
	}
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

func (a *typeArg) validate(scope *Scope) error {
	if boundTo, ok := scope.GetBoundArgInChain(a.typ); ok {
		return boundTo.validate(scope)
	}
	ids := scope.GetServicesIDsByTypeInChain(a.typ)
	if len(ids) == 0 {
		return fmt.Errorf("no services found for type %s", util.Signature(a.typ))
	}
	if !a.slice && len(ids) > 1 {
		return fmt.Errorf("multiple services found for type %s", util.Signature(a.typ))
	}
	return nil
}

func (a *typeArg) resolve(scope *Scope) (any, error) {
	if boundTo, ok := scope.GetBoundArgInChain(a.typ); ok {
		return boundTo.resolve(scope)
	}
	vals, err := scope.GetServicesByTypeInChain(a.typ)
	if err != nil {
		return nil, errorsx.Wrap(err, "failed to resolve type arg")
	}
	if len(vals) == 0 {
		return nil, fmt.Errorf("no services found for type %s", util.Signature(a.typ))
	}
	if a.slice {
		return convertSlice(vals, a.typ)
	}
	if len(vals) > 1 {
		// This should never happen under normal circumstances - the built-in compiler passes verify args.
		return nil, fmt.Errorf("multiple services found for type %s", util.Signature(a.typ))
	}
	return vals[0], nil
}

func (a *typeArg) resolveIDs(scope *Scope) []ID {
	if boundTo, ok := scope.GetBoundArgInChain(a.typ); ok {
		return boundTo.resolveIDs(scope)
	}
	return scope.GetServicesIDsByTypeInChain(a.typ)
}

func (a *typeArg) trace(scope *Scope, path tracePath) ArgTrace {
	t := ArgTrace{Kind: ArgKindType, Bindings: path.hops}
	// a.typ is the element type when the argument collects a slice, and that is
	// what bindings and services are looked up by either way.
	t.matchType(scope, a.typ, path, ResolutionByType)
	return t
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

func (a *labelArg) validate(scope *Scope) error {
	ids := scope.GetServicesIDsByLabelInChain(a.label)
	if len(ids) == 0 {
		return fmt.Errorf("no services found with label %s", a.label)
	}
	if !a.slice && len(ids) > 1 {
		return fmt.Errorf("multiple services found with label %s", a.label)
	}
	return nil
}

func (a *labelArg) resolve(scope *Scope) (any, error) {
	vals, err := scope.GetServicesByLabelInChain(a.label)
	if err != nil {
		return nil, errorsx.Wrap(err, "failed to resolve type arg")
	}
	if len(vals) == 0 {
		return nil, fmt.Errorf("no services found with label %s", a.label)
	}
	if a.slice {
		return convertSlice(vals, a.typ)
	}
	if len(vals) > 1 {
		// This should never happen under normal circumstances - the built-in compiler passes verify args.
		return nil, fmt.Errorf("multiple services found for label %s", a.label)
	}
	argType := reflect.TypeOf(vals[0])
	if argType != a.Type() {
		return nil, fmt.Errorf("service labeled as %s should be of type %s, got %s", a.label, util.Signature(a.Type()), util.Signature(argType))
	}
	return vals[0], nil
}

func (a *labelArg) resolveIDs(scope *Scope) []ID {
	return scope.GetServicesIDsByLabelInChain(a.label)
}

func (a *labelArg) trace(scope *Scope, path tracePath) ArgTrace {
	t := ArgTrace{
		Kind:     ArgKindLabel,
		Bindings: path.hops,
		Label:    a.label,
		Matches:  scope.GetServicesIDsByLabelInChain(a.label),
		By:       ResolutionByLabel,
	}
	if len(t.Matches) == 0 {
		t.Fault = ArgFault{Kind: ArgFaultNoServicesWithLabel, Label: a.label}
	}
	return t
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

// The slice type wins over the element type. Each is checked against the
// bindings first. The three methods below try them in that order.

func (a *flexibleSliceArg) validate(scope *Scope) error {
	// First try to match by the slice type.
	if boundTo, ok := scope.GetBoundArgInChain(a.Type()); ok {
		return boundTo.validate(scope)
	}
	ids := scope.GetServicesIDsByTypeInChain(a.Type())
	if len(ids) > 1 {
		return fmt.Errorf("multiple services found for type %s", util.Signature(a.Type()))
	}
	if len(ids) == 1 {
		return nil // Slice type matched!
	}

	// Now let's try to match by the element type.
	if boundTo, ok := scope.GetBoundArgInChain(a.elemType); ok {
		return boundTo.validate(scope)
	}
	ids = scope.GetServicesIDsByTypeInChain(a.elemType)
	if len(ids) > 0 {
		return nil // Slice element type matched!
	}
	if a.allowEmpty {
		return nil
	}

	return fmt.Errorf("no services found for type %s", util.Signature(a.Type()))
}

func (a *flexibleSliceArg) resolve(scope *Scope) (any, error) {
	// First try to match by the slice type.
	if boundTo, ok := scope.GetBoundArgInChain(a.Type()); ok {
		return boundTo.resolve(scope)
	}
	vals, err := scope.GetServicesByTypeInChain(a.Type())
	if err != nil {
		return nil, errorsx.Wrap(err, "failed to resolve flexible slice arg")
	}
	if len(vals) > 1 {
		// This should never happen under normal circumstances - the built-in compiler passes verify args.
		return nil, fmt.Errorf("multiple services found for type %s", util.Signature(a.Type()))
	}
	if len(vals) == 1 {
		return vals[0], nil // Slice type matched!
	}

	// Now let's try to match by the element type.
	if boundTo, ok := scope.GetBoundArgInChain(a.elemType); ok {
		return boundTo.resolve(scope)
	}
	vals, err = scope.GetServicesByTypeInChain(a.elemType)
	if err != nil {
		return nil, errorsx.Wrap(err, "failed to resolve flexible slice arg element")
	}
	if len(vals) > 0 {
		return convertSlice(vals, a.elemType) // Slice element type matched!
	}
	if a.allowEmpty {
		return convertSlice(nil, a.elemType)
	}

	return nil, fmt.Errorf("no services found for type %s", util.Signature(a.Type()))
}

func (a *flexibleSliceArg) resolveIDs(scope *Scope) []ID {
	// First try to match by the slice type.
	if boundTo, ok := scope.GetBoundArgInChain(a.Type()); ok {
		return boundTo.resolveIDs(scope)
	}
	ids := scope.GetServicesIDsByTypeInChain(a.Type())
	if len(ids) > 0 {
		return ids // Slice type matched!
	}

	// Now let's try to match by the element type.
	if boundTo, ok := scope.GetBoundArgInChain(a.elemType); ok {
		return boundTo.resolveIDs(scope)
	}
	return scope.GetServicesIDsByTypeInChain(a.elemType)
}

func (a *flexibleSliceArg) trace(scope *Scope, path tracePath) ArgTrace {
	t := ArgTrace{Kind: ArgKindFlexibleSlice, Bindings: path.hops}

	// First try to match by the slice type.
	sliceTyp := a.Type()
	if bindScope, binding, ok := scope.bindingInChain(sliceTyp); ok {
		t.follow(scope, sliceTyp, bindScope, binding, path)
		return t
	}
	if ids := scope.GetServicesIDsByTypeInChain(sliceTyp); len(ids) > 0 {
		t.Matches, t.By = ids, ResolutionBySliceType
		return t
	}

	// Now let's try to match by the element type.
	if bindScope, binding, ok := scope.bindingInChain(a.elemType); ok {
		t.follow(scope, a.elemType, bindScope, binding, path)
		return t
	}
	t.Matches, t.By = scope.GetServicesIDsByTypeInChain(a.elemType), ResolutionByElemType
	if len(t.Matches) == 0 && !a.allowEmpty {
		t.Fault = ArgFault{Kind: ArgFaultNoServicesOfType, Type: sliceTyp}
	}
	return t
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

func (c *compoundArg) validate(scope *Scope) error {
	var joinedErr error
	for i, arg := range c.args {
		err := arg.validate(scope)
		if err != nil {
			joinedErr = errors.Join(joinedErr, errorsx.Wrapf(err, "failed to resolve compound sub-arg %d", i))
		}
	}
	return joinedErr
}

func (c *compoundArg) resolve(scope *Scope) (any, error) {
	vals := make([]any, len(c.args))
	for i, arg := range c.args {
		v, err := arg.resolve(scope)
		if err != nil {
			return nil, errorsx.Wrapf(err, "failed to resolve compound sub-arg %d", i)
		}
		vals[i] = v
	}
	return convertSlice(vals, c.typ)
}

func (c *compoundArg) resolveIDs(scope *Scope) []ID {
	return lo.FlatMap(c.args, func(arg Arg, _ int) []ID {
		return arg.resolveIDs(scope)
	})
}

func (c *compoundArg) trace(scope *Scope, path tracePath) ArgTrace {
	t := ArgTrace{Kind: ArgKindCompound, Bindings: path.hops}
	for _, arg := range c.args {
		t.Parts = append(t.Parts, arg.trace(scope, path))
	}
	return t
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

// ArgOrigin tells who filled a Slot. A slot exists before anything fills it, so
// the zero value is ArgOriginNone.
//
// We record origins to tell an argument wired by the user from one wired by godi
// or by a third-party compiler pass. Once the container is built, the three look
// the same.
//
// A pass cannot set an origin. The compiler credits each pass with whatever it
// changed while it ran.
type ArgOrigin uint8

const (
	ArgOriginNone         ArgOrigin = iota // Nothing has filled this slot.
	ArgOriginManual                        // Filled by the user, at definition time.
	ArgOriginAutowiring                    // Filled by the autowiring pass.
	ArgOriginCompilerPass                  // Filled or replaced by a Compiler pass.
)

type Slot struct {
	arg      Arg
	args     []Arg
	typ      reflect.Type
	i        uint
	variadic bool

	origin     ArgOrigin
	originPass string // Name of the pass, for ArgOriginAutowiring and ArgOriginCompilerPass.
	dirty      bool   // Whether the slot was filled since the Compiler last looked.
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
	s.markFilled()
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
	s.markFilled()
	return nil
}

// markFilled records that the slot was just filled. The Compiler credits it
// later.
//
// Until then the fill counts as the user's own. A container that is never
// compiled still reports its wiring correctly.
func (s *Slot) markFilled() {
	s.origin = ArgOriginManual
	s.originPass = ""
	s.dirty = true
}

// creditTo names whoever made the pending fill.
func (s *Slot) creditTo(origin ArgOrigin, pass string) {
	s.origin = origin
	s.originPass = pass
	s.dirty = false
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

// Origin says who filled this slot, and names the pass when one did.
func (s *Slot) Origin() (ArgOrigin, string) {
	return s.origin, s.originPass
}

type ArgList struct {
	slots    Slots
	variadic bool
}

func NewArgList(fnType reflect.Type) *ArgList {
	return &ArgList{
		slots: lo.RepeatBy(fnType.NumIn(), func(i int) *Slot {
			//nolint:gosec // G115: integer overflow conversion int -> uint - no real danger here
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

// Utils

func convertSlice(vs []any, elemType reflect.Type) (any, error) {
	sl := reflect.MakeSlice(reflect.SliceOf(elemType), 0, len(vs))
	for _, v := range vs {
		rv := reflect.ValueOf(v)
		if !rv.Type().AssignableTo(elemType) {
			return nil, fmt.Errorf("type %s is not assignable to %s", util.Signature(rv.Type()), util.Signature(elemType))
		}
		sl = reflect.Append(sl, rv)
	}
	return sl.Interface(), nil
}
