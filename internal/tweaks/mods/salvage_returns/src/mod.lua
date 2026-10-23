-- @tweak name="Salvage returns" group="Crafting"
-- @desc Lets the world's upgrade-recycling setting (runes refunded when
-- @desc salvaging upgraded gear) go above 100%.
-- @param RECYCLING_MAX label="Highest upgrade-recycling setting" min=1 max=4 step=0.25 default=2.0 kind=float
--
-- Part of enshroudedbonker (MIT). A plain EML mod: drop the folder into
-- upstream EML's mods/ and it runs as is.

local RECYCLING_MAX = 2.0

-- The base payout for salvaging gear is not raised here, because no data can
-- raise it alone. In game it behaves as a share of the item's upgrade price: it
-- followed BalancingTable.perkUpgradeCostSeed, the per-type cost factors and the
-- world's upgrade-cost setting, each of which prices upgrades too, and did not
-- follow trashStart / trashEnd. Issue #20.

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
print("salvage_returns: recycling up to " .. RECYCLING_MAX .. " (" .. added .. " slider steps)")
