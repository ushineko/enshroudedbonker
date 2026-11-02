# Double jump research

Recon of 2026-11-02 on game build 1283531 (`release_1.0`), Steam build 26320373,
`enshrouded.exe` sha256 `0403981fd2e8ad18bd8c5698dff03c8d505b5a46677832c496db9c645513f2a6`.

This folder holds game-derived names, ids and addresses, kept here as recon for a
possible future build under a maintainer-approved exception to the public-repository
rule. Addresses are valid for this build only. Disassembly listings stay out of the
repository, in the gitignored `work/double-jump-research/probes/`.

## Result

| Effect | Where it lives | Tested in game |
|---|---|---|
| Stamina cost | `Ability_DoubleJump` item, `impactValues.simple[1]` (vanilla 10) | Works, by EML mod |
| Fall damage | `FallDamage` on templates `1_Player` and `1_Player_AG2` (vanilla start 8, lethal 16) | Works, by EML mod |
| Height | Code in `enshrouded.exe`, not the archive | Works, by live memory patch only |

EML mods cannot reach the height. Patching the exe on disk is not an option: its code
is encrypted by Steam DRM (`.bind` section) and the project treats it as read-only.

## How height is computed

- `0x140421510` builds the jump's velocity vector, switching on `JumpType`.
- Launch speed is `sqrt(2 * h * g)`: `0x140E9F7F0(h, g)`.
- `g` comes from `0x140E8F180`, which returns a constant 25.
- `h` comes from `0x140424840(JumpType)`, a switch with its jump table at `0x140424898`
  (32-bit offsets from the image base 0x140000000, entry = table + 4 * JumpType).

| JumpType | Index | Height |
|---|---|---|
| Default | 1 | 2 |
| DoubleDefault | 2 | 2 |
| DoubleSprint | 3 | 2 |
| DoubleMountain | 4 | 1.25 |
| Sprint | 5 | 2.2 |
| DefaultSubmerged, Glider, HookSwingEnter, LadderClimbing, FreeClimbing, Slide, NpcMovement | 6, 8, 9, 12, 13, 15, 18 | 2 |
| HookSwingDefaultExit, HookSwingSprintExit | 10, 11 | 0.72 |
| Mountain | 14 | 1.25 |
| Attack | 16 | 1 |
| Airwolf | 17 | 0.4 |
| Invalid, StruggleSubmerged, JumpPad | 0, 7, 19 | 0 |

The case constants are shared with unrelated code (2.2 has several other readers, and
2 is used widely). Change the table entry or the case, never the constant.

Related, not height:

- `0x140422780` runs the jump and is called from `actor_jump`.
- The table at `0x140422C84` writes `DynamicLocomotion + 0x174`. By the schema's field
  order that is `airControlMaxVelocity`. Changing it had no visible effect on height.
- `0x1404232C0` and `0x140423360` return per-type values of 13 and 14.857 for most
  types. They are probably rising and falling gravity. That is unconfirmed.
- `0x1404247D0` returns 8 or 4.15, meaning unknown.

## What did not work

On `Ability_Movement_Skill_JumpDoubleSecond` (guid `e7b5dbb5-db37-4efa-ba2e-8e94c3f35fba`),
all tested in game, none with a visible effect on the jump:

- `ApplyImpulseEvent`, up 6 and then 30
- `ImpulseEvent` up 40, fading over 1.4 s, plus `SetGravityEvent` x0.25
- Moving the existing `JumpEvent` to 0.5 s, and movement factors x5
- Table `0x140422C84` for DoubleDefault pointed at Sprint's 9.5

The edited sequence was in memory, and the game's resource tables pointed at it. Why
those events do nothing was not settled. Two memory watches for the sequence's id
(`tools/watch.py`, `tools/watch2.py`) saw nothing, and neither had a positive control,
so they do not show whether the sequence runs.

## Finding things again after a game update

1. `.text` is encrypted on disk, so read it live: `/proc/<pid>/mem`, no sudo needed here.
2. System names are not reached by `lea`. `.rdata` has tables of name pointer, length,
   function pointer. `tools/ptrs.py <pid> actor_jump ...` lists them.
3. Follow `actor_jump` to the `cmp r12d,13` function, then to the `cmp r15b,13` velocity
   function, then to the height switch: `cmp cl,13` then a table jump, returning floats
   including 2, 2.2 and 1.25.
4. `0x13` is the last `JumpType`. `keen.actor.JumpType` in emm's generated `types.lua`
   gives the order. Check it has not changed.
5. Update the addresses and `HEAD` in `ce/double_jump_height.lua`.

## Files

- `ce/double_jump_height.lua`: the patch. It allocates a new case with its own constant,
  and points DoubleDefault and DoubleSprint at it, so Sprint and all else stay vanilla.
  It checks the function's bytes first and refuses on any other build. Height and
  whether to include the sprint double jump are set at the top.
- `ce/disasm_helper.lua`: CE autorun script for disassembly and guarded writes by file.
- `tools/`: read-only memory scanners, and `ask.py`, which talks to the helper.
- `experiment-mod/`: the last EML experiment. It holds the control test (delay and
  speed edits, cost 1, fall damage x10), not a finished tweak.
- `probes/`: the last probe mod.

## Running Cheat Engine

CE is not stored here. Copy `Program Files/Cheat Engine` from another Proton prefix (for
example `compatdata/1942280/pfx/drive_c/`), then with the game running:

    WINEPREFIX=<steam>/steamapps/compatdata/1203620/pfx/ WINEDEBUG=-all \
      "<steam>/steamapps/common/Proton - Experimental/files/bin/wine" ./cheatengine-x86_64.exe

That joins the game's wineserver, so `openProcess("enshrouded.exe")` works. It has to be
the Proton the game is running under. Then run the script from CE's Lua Engine, or drop
it into CE's `autorun/`. The patch stays after CE closes and ends when the game exits.

## Test status

- Tested in game: height through an earlier version of the patch (a repointed Sprint
  case at 6.5). Second jump clearly higher.
- Tested by memory read only: `ce/double_jump_height.lua` apply, restore and apply again.
  The table and case hold the right values. Not yet jumped with in game.
- Not re-run: `ce/disasm_helper.lua` as saved here. It is the helper used during
  the research, retyped with a placeholder `DIR`.
- Not tested: multiplayer. Only the player's own local server was used.
