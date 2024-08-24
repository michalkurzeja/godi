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

type Arg interface {
	fmt.Stringer
	Type() reflect.Type
}

type ArgResolver interface {
	ResolveIDs(arg Arg) []ID
	Resolve(arg Arg) (any, error)
	Validate(arg Arg) error
}

type argResolver struct {
	container *Container

	literalArgResolver       *literalArgResolver
	refArgResolver           *refArgResolver
	typeArgResolver          *typeArgResolver
	labelArgResolver         *labelArgResolver
	flexibleSliceArgResolver *flexibleSliceArgResolver
	compoundArgResolver      *compoundArgResolver
}

func NewArgResolver(container *Container) ArgResolver {
	r := &argResolver{container: container}
	r.literalArgResolver = &literalArgResolver{}
	r.refArgResolver = &refArgResolver{container: container}
	r.typeArgResolver = &typeArgResolver{resolver: r, container: container}
	r.labelArgResolver = &labelArgResolver{resolver: r, container: container}
	r.flexibleSliceArgResolver = &flexibleSliceArgResolver{resolver: r, container: container}
	r.compoundArgResolver = &compoundArgResolver{resolver: r, container: container}
	return r
}

func (r *argResolver) Validate(arg Arg) error {
	switch a := arg.(type) {
	case *literalArg:
		return r.literalArgResolver.Validate(a)
	case *refArg:
		return r.refArgResolver.Validate(a)
	case *typeArg:
		return r.typeArgResolver.Validate(a)
	case *labelArg:
		return r.labelArgResolver.Validate(a)
	case *flexibleSliceArg:
		return r.flexibleSliceArgResolver.Validate(a)
	case *compoundArg:
		return r.compoundArgResolver.Validate(a)
	default:
		return fmt.Errorf("unsupported arg type %T", arg)
	}
}

func (r *argResolver) Resolve(arg Arg) (any, error) {
	switch a := arg.(type) {
	case *literalArg:
		return r.literalArgResolver.Resolve(a)
	case *refArg:
		return r.refArgResolver.Resolve(a)
	case *typeArg:
		return r.typeArgResolver.Resolve(a)
	case *labelArg:
		return r.labelArgResolver.Resolve(a)
	case *flexibleSliceArg:
		return r.flexibleSliceArgResolver.Resolve(a)
	case *compoundArg:
		return r.compoundArgResolver.Resolve(a)
	default:
		return reflect.Value{}, fmt.Errorf("unsupported arg type %T", arg)
	}
}

func (r *argResolver) ResolveIDs(arg Arg) []ID {
	switch a := arg.(type) {
	case *literalArg:
		return r.literalArgResolver.ResolveIDs(a)
	case *refArg:
		return r.refArgResolver.ResolveIDs(a)
	case *typeArg:
		return r.typeArgResolver.ResolveIDs(a)
	case *labelArg:
		return r.labelArgResolver.ResolveIDs(a)
	case *flexibleSliceArg:
		return r.flexibleSliceArgResolver.ResolveIDs(a)
	case *compoundArg:
		return r.compoundArgResolver.ResolveIDs(a)
	default:
		return nil
	}
}

// literal arg

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

type literalArgResolver struct{}

func (r *literalArgResolver) Validate(_ *literalArg) error {
	return nil
}

func (r *literalArgResolver) Resolve(a *literalArg) (any, error) {
	return a.v, nil
}

func (r *literalArgResolver) ResolveIDs(_ *literalArg) []ID {
	return nil
}

// ref arg

type refArg struct {
	def *ServiceDefinition
}

func NewRefArg(def *ServiceDefinition) Arg {
	return &refArg{def: def}
}

func (a *refArg) String() string {
	return a.def.String()
}

func (a *refArg) Type() reflect.Type {
	return a.def.Type()
}

type refArgResolver struct{ container *Container }

func (r *refArgResolver) Validate(a *refArg) error {
	if !r.container.HasService(a.def.ID()) {
		return fmt.Errorf("service %s not found", a.def.ID())
	}
	return nil
}

func (r *refArgResolver) Resolve(a *refArg) (any, error) {
	v, err := r.container.GetService(a.def.ID())
	if err != nil {
		return nil, errorsx.Wrap(err, "failed to resolve ID arg")
	}
	if v == nil {
		return nil, fmt.Errorf("service %s not found", a.def.ID())
	}
	return v, nil
}

func (r *refArgResolver) ResolveIDs(a *refArg) []ID {
	return []ID{a.def.ID()}
}

// type arg

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

type typeArgResolver struct {
	resolver  *argResolver
	container *Container
}

func (r *typeArgResolver) Validate(a *typeArg) error {
	if boundTo, ok := r.container.GetBindingFor(a.typ); ok {
		return r.resolver.Validate(boundTo)
	}
	ids := r.container.GetServicesIDsByType(a.typ)
	if len(ids) == 0 {
		return fmt.Errorf("no services found for type %s", util.Signature(a.typ))
	}
	if !a.slice && len(ids) > 1 {
		return fmt.Errorf("multiple services found for type %s", util.Signature(a.typ))
	}
	return nil
}

func (r *typeArgResolver) Resolve(a *typeArg) (any, error) {
	if boundTo, ok := r.container.GetBindingFor(a.typ); ok {
		return r.resolver.Resolve(boundTo)
	}
	vals, err := r.container.GetServicesByType(a.typ)
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

func (r *typeArgResolver) ResolveIDs(a *typeArg) []ID {
	if boundTo, ok := r.container.GetBindingFor(a.typ); ok {
		return r.resolver.ResolveIDs(boundTo)
	}
	return r.container.GetServicesIDsByType(a.typ)
}

// label arg

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

type labelArgResolver struct {
	resolver  *argResolver
	container *Container
}

func (r *labelArgResolver) Validate(a *labelArg) error {
	ids := r.container.GetServicesIDsByLabel(a.label)
	if len(ids) == 0 {
		return fmt.Errorf("no services found with label %s", a.label)
	}
	if !a.slice && len(ids) > 1 {
		return fmt.Errorf("multiple services found with label %s", a.label)
	}
	return nil
}

func (r *labelArgResolver) Resolve(a *labelArg) (any, error) {
	vals, err := r.container.GetServicesByLabel(a.label)
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

func (r *labelArgResolver) ResolveIDs(a *labelArg) []ID {
	return r.container.GetServicesIDsByLabel(a.label)
}

// flexible slice arg

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

type flexibleSliceArgResolver struct {
	resolver  *argResolver
	container *Container
}

func (r *flexibleSliceArgResolver) Validate(a *flexibleSliceArg) error {
	// First try to match by the slice type.
	if boundTo, ok := r.container.GetBindingFor(a.Type()); ok {
		return r.resolver.Validate(boundTo)
	}
	ids := r.container.GetServicesIDsByType(a.Type())
	if len(ids) > 1 {
		return fmt.Errorf("multiple services found for type %s", util.Signature(a.Type()))
	}
	if len(ids) == 1 {
		return nil // Slice type matched!
	}

	// Now let's try to match by the element type.
	elemType := a.Type().Elem()
	if boundTo, ok := r.container.GetBindingFor(elemType); ok {
		return r.resolver.Validate(boundTo)
	}
	ids = r.container.GetServicesIDsByType(elemType)
	if len(ids) > 0 {
		return nil // Slice element type matched!
	}
	if a.allowEmpty {
		return nil
	}

	return fmt.Errorf("no services found for type %s", util.Signature(a.Type()))
}

func (r *flexibleSliceArgResolver) Resolve(a *flexibleSliceArg) (any, error) {
	// First try to match by the slice type.
	if boundTo, ok := r.container.GetBindingFor(a.Type()); ok {
		return r.resolver.Resolve(boundTo)
	}
	vals, err := r.container.GetServicesByType(a.Type())
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
	elemType := a.Type().Elem()
	if boundTo, ok := r.container.GetBindingFor(elemType); ok {
		return r.resolver.Resolve(boundTo)
	}
	vals, err = r.container.GetServicesByType(elemType)
	if err != nil {
		return nil, errorsx.Wrap(err, "failed to resolve flexible slice arg element")
	}
	if len(vals) > 0 {
		return convertSlice(vals, elemType) // Slice element type matched!
	}
	if a.allowEmpty {
		return convertSlice(nil, elemType)
	}

	return nil, fmt.Errorf("no services found for type %s", util.Signature(a.Type()))
}

func (r *flexibleSliceArgResolver) ResolveIDs(a *flexibleSliceArg) []ID {
	// First try to match by the slice type.
	if boundTo, ok := r.container.GetBindingFor(a.Type()); ok {
		return r.resolver.ResolveIDs(boundTo)
	}
	ids := r.container.GetServicesIDsByType(a.Type())
	if len(ids) > 0 {
		return ids // Slice type matched!
	}

	// Now let's try to match by the element type.
	elemType := a.Type().Elem()
	if boundTo, ok := r.container.GetBindingFor(elemType); ok {
		return r.resolver.ResolveIDs(boundTo)
	}
	return r.container.GetServicesIDsByType(elemType)
}

// compound arg

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

type compoundArgResolver struct {
	resolver  *argResolver
	container *Container
}

func (r *compoundArgResolver) Validate(a *compoundArg) error {
	var joinedErr error
	for i, arg := range a.args {
		err := r.resolver.Validate(arg)
		if err != nil {
			joinedErr = errors.Join(joinedErr, errorsx.Wrapf(err, "failed to resolve compound sub-arg %d", i))
		}
	}
	return joinedErr
}

func (r *compoundArgResolver) Resolve(a *compoundArg) (any, error) {
	vals := make([]any, len(a.args))
	for i, arg := range a.args {
		v, err := r.resolver.Resolve(arg)
		if err != nil {
			return nil, errorsx.Wrapf(err, "failed to resolve compound sub-arg %d", i)
		}
		vals[i] = v
	}
	return convertSlice(vals, a.typ)
}

func (r *compoundArgResolver) ResolveIDs(a *compoundArg) []ID {
	return lo.FlatMap(a.args, func(a Arg, _ int) []ID {
		return r.resolver.ResolveIDs(a)
	})
}

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
