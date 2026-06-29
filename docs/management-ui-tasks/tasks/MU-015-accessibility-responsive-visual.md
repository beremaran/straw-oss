# MU-015: Accessibility, Responsive Behavior, And Visual Design Pass

Status: not-started
Phase: 4
Depends on: MU-006 through MU-014
Search tags: accessibility, responsive, keyboard, mobile cards, charts, aria-label, focus, visual design

## Objective

Verify and finish the first-release UI against the spec accessibility, responsive, and operational-console design requirements.

## Scope

- Ensure all controls have visible labels.
- Add `aria-label` and tooltips for icon-only actions.
- Move focus into destructive dialogs and restore focus to the trigger after close.
- Expose table row headers and selected state to assistive technology.
- Provide data table alternatives for charts.
- Pair status colors with text labels.
- Verify keyboard support for navigation, menus, dialogs, chip editors, and JSON editor escape behavior.
- Verify desktop, tablet, and mobile layouts for navigation, forms, tables, cards, and sticky footers.
- Keep visual design dense, neutral, and operational; avoid marketing-style hero sections and decorative cards.

## Repo Touchpoints

- `web/management/src/components/*`
- `web/management/src/routes/*`
- app-wide styles/theme files

## Implementation Tasks

- [ ] Run keyboard-only pass across all routes.
- [ ] Check screen-reader-visible labels for all controls and icon buttons.
- [ ] Verify mobile card replacements for dense tables.
- [ ] Verify responsive form section order and sticky footer behavior.
- [ ] Fix text overflow, overlapping controls, and color-only status indicators.

## Done Criteria

- [ ] Accessibility requirements in `docs/management-ui-spec.md` are satisfied.
- [ ] Desktop, tablet, and mobile layouts expose the same first-release actions.
- [ ] Destructive actions remain behind confirmation at every viewport size.
- [ ] Charts have same-page table alternatives.

