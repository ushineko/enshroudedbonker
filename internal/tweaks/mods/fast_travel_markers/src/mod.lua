-- @tweak name="Fast-travel destinations" group="World"
-- @desc Makes dungeon and location markers on the map fast-travel destinations.
-- @desc It does not lift the in-Shroud, falling or underwater blocks. Travelling to
-- @desc a marker can land you outside the playable map or leave a gravestone
-- @desc unreachable.
-- @param DUNGEONS label="Dungeons" min=0 max=1 step=1 default=1 kind=bool
-- @param LOCATIONS label="Locations" min=0 max=1 step=1 default=1 kind=bool
--
-- Part of enshroudedbonker (MIT). A plain EML mod: drop the folder into
-- upstream EML's mods/ and it runs as is.

local DUNGEONS = true
local LOCATIONS = true

local WANT = { Dungeons = DUNGEONS, Locations = LOCATIONS }
local reg = game.assets.get_resources_by_type("keen::MapMarkerRegistryResource")[1].data
local n = 0
for _, m in ipairs(reg.mapMarkers.values) do
  if WANT[m.sortingCategory] and not m.isFastTravelDestination then
    m.isFastTravelDestination = true
    n = n + 1
  end
end
print("fast_travel_markers: enabled " .. n .. " marker types")
