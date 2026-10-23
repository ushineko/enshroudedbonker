-- @tweak name="Rune drops" group="Loot"
-- @desc Adds runes to what enemies leave behind to loot, humanoids and wildlife
-- @desc alike, on top of their usual loot. The world's loot-amount setting scales
-- @desc the amount, as it does every drop.
-- @param RUNES_MIN label="Fewest runes" min=1 max=100 step=1 default=2
-- @param RUNES_MAX label="Most runes" min=1 max=100 step=1 default=5
-- @param CHANCE label="Chance per corpse" min=0.05 max=1 step=0.05 default=1.0 kind=float
--
-- Part of enshroudedbonker (MIT). A plain EML mod: drop the folder into
-- upstream EML's mods/ and it runs as is.

local RUNES_MIN = 2
local RUNES_MAX = 5
local CHANCE = 1.0

-- An enemy's loot is a separate template, LootContainer_Enemy_*, holding a fixed
-- drop list. The list's root group is plain (not a selector), so an added stack
-- drops alongside the rest. A list attached to the enemy template itself is
-- ignored on death, which is why the corpse is the one changed. Spec 009.
local lo = math.min(RUNES_MIN, RUNES_MAX)

-- The rune vendor upgrades charge.
local bt = game.assets.get_resources_by_type("keen::BalancingTable")[1].data
local runeRef = bt.perkCostItem
local runeId = game.assets.get_resource(runeRef, "keen::ItemInfo").data.itemId.value

-- Lists are shared between corpse templates; each gets one stack.
local seen, lists, corpses = {}, 0, 0
for _, r in ipairs(game.assets.get_resources_by_type("keen::ecs::TemplateResource")) do
  if string.match(r.data.name, "^LootContainer_Enemy_") then
    for _, comp in ipairs(r.data.components) do
      if comp.type == "keen::ecs::DefaultInventory" then
        local inv = game.assets.get_resource(comp.value.defaultInventory, "keen::ecs::DefaultInventoryResource")
        if inv ~= nil then
          corpses = corpses + 1
          local key = tostring(inv.guid)
          if not seen[key] then
            seen[key] = true
            table.insert(inv.data.rootGroup.stacks, {
              itemRef = runeRef, item = { value = runeId },
              countMin = lo, countMax = RUNES_MAX, spawnProbability = CHANCE, weight = 1.0,
              lootLabelsId = { value = 0 },
            })
            lists = lists + 1
          end
        end
      end
    end
  end
end
print("rune_drops: " .. lo .. "-" .. RUNES_MAX .. " runes at " .. CHANCE .. " added to " .. lists ..
  " drop lists (" .. corpses .. " enemy corpses)")
