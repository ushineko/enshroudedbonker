-- @tweak name="Class loot weights" group="Loot"
-- @desc Thins each class's weapons, armor, rings and shields out of random loot,
-- @desc so the classes you keep drop more often. A weight of 1 leaves a class as
-- @desc it is; lower weights pull that share of its gear from random drops,
-- @desc keeping at least one of each kind. The default levels the weapon pool,
-- @desc which is about half warrior gear.
-- @param WARRIOR_WEIGHT label="Warrior gear" min=0 max=1 step=0.05 default=0.4 kind=float
-- @param RANGER_WEIGHT label="Ranger gear" min=0 max=1 step=0.05 default=1.0 kind=float
-- @param MAGE_WEIGHT label="Mage gear" min=0 max=1 step=0.05 default=1.0 kind=float
--
-- Part of enshroudedbonker (MIT). A plain EML mod: drop the folder into
-- upstream EML's mods/ and it runs as is.

local WARRIOR_WEIGHT = 0.4
local RANGER_WEIGHT = 1.0
local MAGE_WEIGHT = 1.0

-- Random gear picks a slot (weapon, armor, ring, shield), then an entry of that
-- shufflebagType from the pool. There is no per-item weight, so a class is
-- weighted by taking some of its entries out of the draw: shufflebagType None
-- with no loot labels never rolls. Entries stay in the pool for lookups by id.
-- Spec 012.
local weights = { Warrior = WARRIOR_WEIGHT, Ranger = RANGER_WEIGHT, Mage = MAGE_WEIGHT }
local classes = { "Warrior", "Ranger", "Mage" }

local db = game.assets.get_resources_by_type("keen::LootableItemsResource")[1].data

-- Group gear by slot, weapon category, armor slot and class, in pool order.
local groups, order = {}, {}
for _, it in ipairs(db.items) do
  local slot = tostring(it.shufflebagType)
  local class = tostring(it.classAlignmentType)
  if slot ~= "None" and weights[class] then
    local key = table.concat({ slot, tostring(it.weaponCategoryType), tostring(it.armorCategoryType), class }, "|")
    if not groups[key] then
      groups[key] = { slot = slot, class = class, items = {} }
      table.insert(order, key)
    end
    table.insert(groups[key].items, it)
  end
end

-- Keep round(n * weight) of each group, at least one, spread evenly through it.
local before, after = {}, {}
for _, c in ipairs(classes) do before[c], after[c] = 0, 0 end
local pulled = 0
for _, key in ipairs(order) do
  local g = groups[key]
  local n = #g.items
  local keep = n
  if weights[g.class] < 1 then
    keep = math.max(1, math.min(n, math.floor(n * weights[g.class] + 0.5)))
  end
  for i, it in ipairs(g.items) do
    if math.floor(i * keep / n) == math.floor((i - 1) * keep / n) then
      it.shufflebagType = "None"
      pulled = pulled + 1
    end
  end
  if g.slot == "Weapon" then
    before[g.class] = before[g.class] + n
    after[g.class] = after[g.class] + keep
  end
end

print(string.format(
  "class_loot_weights: warrior x%.2f, ranger x%.2f, mage x%.2f; weapons %d/%d/%d -> %d/%d/%d; %d entries out of random loot",
  WARRIOR_WEIGHT, RANGER_WEIGHT, MAGE_WEIGHT,
  before.Warrior, before.Ranger, before.Mage, after.Warrior, after.Ranger, after.Mage, pulled))
