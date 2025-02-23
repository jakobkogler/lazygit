package gui

import (
	"fmt"

	"github.com/jesseduffield/gocui"
)

type ListContext struct {
	GetItemsLength      func() int
	GetDisplayStrings   func(startIdx int, length int) [][]string
	OnFocus             func() error
	OnFocusLost         func() error
	OnClickSelectedItem func() error

	// the boolean here tells us whether the item is nil. This is needed because you can't work it out on the calling end once the pointer is wrapped in an interface (unless you want to use reflection)
	SelectedItem    func() (ListItem, bool)
	OnGetPanelState func() IListPanelState
	// if this is true, we'll call GetDisplayStrings for just the visible part of the
	// view and re-render that. This is useful when you need to render different
	// content based on the selection (e.g. for showing the selected commit)
	RenderSelection bool

	Gui *Gui

	*BasicContext
}

type IListContext interface {
	GetSelectedItem() (ListItem, bool)
	GetSelectedItemId() string
	OnRender() error
	handlePrevLine() error
	handleNextLine() error
	handleScrollLeft() error
	handleScrollRight() error
	handleLineChange(change int) error
	handleNextPage() error
	handleGotoTop() error
	handleGotoBottom() error
	handlePrevPage() error
	handleClick() error
	onSearchSelect(selectedLineIdx int) error
	FocusLine()

	GetPanelState() IListPanelState

	Context
}

func (self *ListContext) GetPanelState() IListPanelState {
	return self.OnGetPanelState()
}

type IListPanelState interface {
	SetSelectedLineIdx(int)
	GetSelectedLineIdx() int
}

type ListItem interface {
	// ID is a SHA when the item is a commit, a filename when the item is a file, 'stash@{4}' when it's a stash entry, 'my_branch' when it's a branch
	ID() string

	// Description is something we would show in a message e.g. '123as14: push blah' for a commit
	Description() string
}

func (self *ListContext) FocusLine() {
	view, err := self.Gui.g.View(self.ViewName)
	if err != nil {
		// ignoring error for now
		return
	}

	// we need a way of knowing whether we've rendered to the view yet.
	view.FocusPoint(view.OriginX(), self.GetPanelState().GetSelectedLineIdx())
	if self.RenderSelection {
		_, originY := view.Origin()
		displayStrings := self.GetDisplayStrings(originY, view.InnerHeight())
		self.Gui.renderDisplayStringsAtPos(view, originY, displayStrings)
	}
	view.Footer = formatListFooter(self.GetPanelState().GetSelectedLineIdx(), self.GetItemsLength())
}

func formatListFooter(selectedLineIdx int, length int) string {
	return fmt.Sprintf("%d of %d", selectedLineIdx+1, length)
}

func (self *ListContext) GetSelectedItem() (ListItem, bool) {
	return self.SelectedItem()
}

func (self *ListContext) GetSelectedItemId() string {
	item, ok := self.GetSelectedItem()

	if !ok {
		return ""
	}

	return item.ID()
}

// OnFocus assumes that the content of the context has already been rendered to the view. OnRender is the function which actually renders the content to the view
func (self *ListContext) OnRender() error {
	view, err := self.Gui.g.View(self.ViewName)
	if err != nil {
		return nil
	}

	if self.GetDisplayStrings != nil {
		self.Gui.refreshSelectedLine(self.GetPanelState(), self.GetItemsLength())
		self.Gui.renderDisplayStrings(view, self.GetDisplayStrings(0, self.GetItemsLength()))
		self.Gui.render()
	}

	return nil
}

func (self *ListContext) HandleFocusLost() error {
	if self.OnFocusLost != nil {
		return self.OnFocusLost()
	}

	view, err := self.Gui.g.View(self.ViewName)
	if err != nil {
		return nil
	}

	_ = view.SetOriginX(0)

	return nil
}

func (self *ListContext) HandleFocus() error {
	if self.Gui.popupPanelFocused() {
		return nil
	}

	self.FocusLine()

	if self.Gui.State.Modes.Diffing.Active() {
		return self.Gui.renderDiff()
	}

	if self.OnFocus != nil {
		return self.OnFocus()
	}

	return nil
}

func (self *ListContext) HandleRender() error {
	return self.OnRender()
}

func (self *ListContext) handlePrevLine() error {
	return self.handleLineChange(-1)
}

func (self *ListContext) handleNextLine() error {
	return self.handleLineChange(1)
}

func (self *ListContext) handleScrollLeft() error {
	return self.scroll(self.Gui.scrollLeft)
}

func (self *ListContext) handleScrollRight() error {
	return self.scroll(self.Gui.scrollRight)
}

func (self *ListContext) scroll(scrollFunc func(*gocui.View)) error {
	if self.ignoreKeybinding() {
		return nil
	}

	// get the view, move the origin
	view, err := self.Gui.g.View(self.ViewName)
	if err != nil {
		return nil
	}

	scrollFunc(view)

	return self.HandleFocus()
}

func (self *ListContext) ignoreKeybinding() bool {
	return !self.Gui.isPopupPanel(self.ViewName) && self.Gui.popupPanelFocused()
}

func (self *ListContext) handleLineChange(change int) error {
	if self.ignoreKeybinding() {
		return nil
	}

	selectedLineIdx := self.GetPanelState().GetSelectedLineIdx()
	if (change < 0 && selectedLineIdx == 0) || (change > 0 && selectedLineIdx == self.GetItemsLength()-1) {
		return nil
	}

	self.Gui.changeSelectedLine(self.GetPanelState(), self.GetItemsLength(), change)

	return self.HandleFocus()
}

func (self *ListContext) handleNextPage() error {
	view, err := self.Gui.g.View(self.ViewName)
	if err != nil {
		return nil
	}
	delta := self.Gui.pageDelta(view)

	return self.handleLineChange(delta)
}

func (self *ListContext) handleGotoTop() error {
	return self.handleLineChange(-self.GetItemsLength())
}

func (self *ListContext) handleGotoBottom() error {
	return self.handleLineChange(self.GetItemsLength())
}

func (self *ListContext) handlePrevPage() error {
	view, err := self.Gui.g.View(self.ViewName)
	if err != nil {
		return nil
	}

	delta := self.Gui.pageDelta(view)

	return self.handleLineChange(-delta)
}

func (self *ListContext) handleClick() error {
	if self.ignoreKeybinding() {
		return nil
	}

	view, err := self.Gui.g.View(self.ViewName)
	if err != nil {
		return nil
	}

	prevSelectedLineIdx := self.GetPanelState().GetSelectedLineIdx()
	newSelectedLineIdx := view.SelectedLineIdx()

	// we need to focus the view
	if err := self.Gui.pushContext(self); err != nil {
		return err
	}

	if newSelectedLineIdx > self.GetItemsLength()-1 {
		return nil
	}

	self.GetPanelState().SetSelectedLineIdx(newSelectedLineIdx)

	prevViewName := self.Gui.currentViewName()
	if prevSelectedLineIdx == newSelectedLineIdx && prevViewName == self.ViewName && self.OnClickSelectedItem != nil {
		return self.OnClickSelectedItem()
	}
	return self.HandleFocus()
}

func (self *ListContext) onSearchSelect(selectedLineIdx int) error {
	self.GetPanelState().SetSelectedLineIdx(selectedLineIdx)
	return self.HandleFocus()
}
