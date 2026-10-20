-- @tweak name="Salvage returns" group="Crafting"
-- @desc More runes from salvaging gear, and lets the world's upgrade-recycling
-- @desc setting (runes refunded when salvaging upgraded gear) go above 100%.
-- @param SALVAGE_MULTIPLIER label="Runes from salvage" min=1 max=10 step=1 default=2
-- @param RECYCLING_MAX label="Highest upgrade-recycling setting" min=1 max=4 step=0.25 default=2.0 kind=float
--
-- Part of enshroudedbonker (MIT). A plain EML mod: drop the folder into
-- upstream EML's mods/ and it runs as is.

local SALVAGE_MULTIPLIER = 2
local RECYCLING_MAX = 2.0

-- Salvaging gear: BalancingTable.trashStart / trashEnd (vanilla 5 and 20) are
-- taken to be the low and high end of the rune payout ("trash" is the game's
-- internal word for salvaging from the backpack).
local bt = game.assets.get_resources_by_type("keen::BalancingTable")[1].data
local t0, t1 = bt.trashStart, bt.trashEnd
bt.trashStart = math.floor(t0 * SALVAGE_MULTIPLIER)   -- u32
bt.trashEnd = math.floor(t1 * SALVAGE_MULTIPLIER)

-- Salvaging upgraded gear also refunds runes: (runes spent on upgrades) x the
-- world setting perkUpgradeRecyclingFactor (Default 0.5, Relaxed 1.0, max 1.0).
-- Raise the allowed maximum and add the higher steps to its slider, whose
-- steps run from high to low.
local presets = game.assets.get_resources_by_type("keen::GameSettingsPresetsResource")[1].data
presets.maxValues.perkUpgradeRecyclingFactor = RECYCLING_MAX
local added = 0
for _, r in ipairs(game.assets.get_resources_by_type("keen::FbUiBundle")) do
  local ok, ds = pcall(function() return r.data.difficultySettings end)
  if ok and ds ~= nil then
    local steps = ds.settingValues.perkUpgradeRecyclingFactor.steps
    local v = 1.25
    local extra = {}
    while v <= RECYCLING_MAX + 1e-6 do
      table.insert(extra, 1, v)
      v = v + 0.25
    end
    for i = #extra, 1, -1 do
      if extra[i] > steps[1] + 1e-6 then
        table.insert(steps, 1, extra[i])
        added = added + 1
      end
    end
  end
end
print("salvage_returns: runes " .. t0 .. "-" .. t1 .. " -> " .. bt.trashStart .. "-" .. bt.trashEnd ..
  "; recycling up to " .. RECYCLING_MAX .. " (" .. added .. " slider steps)")
