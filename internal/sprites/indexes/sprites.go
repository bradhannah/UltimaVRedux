package indexes

// StandardNumberOfAnimationFrames is the frame count of a standard animation
// sequence, counted from its key index.
const StandardNumberOfAnimationFrames = 4

// SpriteIndex identifies one sprite in the shared atlas; NoSprites marks none.
type SpriteIndex int

// Sprite identities indexing the shared sprite atlas, each named for the
// original Ultima V tile it shows.
const (
	NoSprites                       SpriteIndex = -1
	Explosion                                   = 0
	Water1                                      = 1
	Water2                                      = 2
	WaterCoast                                  = 3
	Swamp                                       = 4
	Grass                                       = 5
	Forest1                                     = 6
	Desert1                                     = 7
	Forest2                                     = 8
	Forest3                                     = 9
	Forest4                                     = 10
	Hills1                                      = 11
	SmallMountains                              = 12
	TallMountains                               = 13
	LeftHills2                                  = 14
	RightHills2                                 = 15
	Hut                                         = 16
	Codex                                       = 17
	Keep                                        = 18
	Village                                     = 19
	SmallCastle                                 = 20
	LargeCastle                                 = 21
	CaveEntrance                                = 22
	MineEntrance                                = 23
	DoomEntrance                                = 24
	Shrine                                      = 25
	BrokenShrine                                = 26
	Lighthouse                                  = 27
	Oasis                                       = 28
	Bridge                                      = 29
	LeftDesert2                                 = 30
	RightDesert2                                = 31
	Path1                                       = 32
	Path2                                       = 33
	Path3                                       = 34
	Path4                                       = 35
	Path5                                       = 36
	Path6                                       = 37
	Path7                                       = 38
	Roof1                                       = 39
	Roof2                                       = 40
	CrystalBall                                 = 41
	LighthouseLight                             = 42
	DeadTree                                    = 43
	PlowedField                                 = 44
	WheatInField                                = 45
	Tree                                        = 46
	Cactus                                      = 47
	OutsidePath1                                = 48
	OutsidePath2                                = 49
	OutsidePath3                                = 50
	OutsidePath4                                = 51
	OutsideWaterCoast1                          = 52
	OutsideWaterCoast2                          = 53
	OutsideWaterCoast3                          = 54
	OutsideWaterCoast4                          = 55
	CastleGargoyle                              = 56
	EvilCastleEntrance                          = 57
	CastleBritian1                              = 58
	CastleBritian2                              = 59
	CastleBritian3                              = 60
	CastleBritian4                              = 61
	CastleBritianEntrace                        = 62
	CastleBritian5                              = 63
	WoodFloor1                                  = 64
	Lecturn                                     = 65
	WoodFloorShipTie                            = 66
	WoodFloorShipEdge                           = 67
	BrickFloor                                  = 68
	MetalFloor                                  = 69
	DryStone                                    = 70
	Dock                                        = 71
	LeftWoodFloor2                              = 72
	RightWoodFloor2                             = 73
	StoneCrossWindow                            = 74
	StoneGlassWindow                            = 75
	SmallRockWall                               = 76
	LargeRockWall                               = 77
	StoneBrickWallSecret                        = 78
	StoneBrickWall                              = 79
	CastleParapet1                              = 80
	CastleParapet2                              = 81
	CastleParapet3                              = 82
	CastleParapet4                              = 83
	CastleParapet5                              = 84
	CastleParapet6                              = 85
	CastleParapet7                              = 86
	CastleParapet8                              = 87
	Anvil                                       = 88
	Telescope                                   = 89
	Bookcase                                    = 90
	Plant                                       = 91
	BookcaseLeft                                = 92
	BookcaseRight                               = 93
	CodexAngelLeft                              = 94
	CodexAngelRight                             = 95
	WaterStream1                                = 96
	WaterStream2                                = 97
	WaterStream3                                = 98
	WaterStream4                                = 99
	WaterStream5                                = 100
	WaterStream6                                = 101
	WaterStream7                                = 102
	WaterStream8                                = 103
	WaterStream9                                = 104
	WaterStream10                               = 105
	TrollBridgeHoriz                            = 106
	TrollBridgeVert                             = 107
	WaterStream11                               = 108
	WaterStream12                               = 109
	WaterStream13                               = 110
	WaterStream14                               = 111
	ShadowlordBoundary1                         = 112
	ShadowlordBoundary2                         = 113
	ShadowlordBoundary3                         = 114
	ShadowlordBoundary4                         = 115
	ShadowlordBoundary5                         = 116
	ShadowlordBoundary6                         = 117
	ShadowlordBoundary7                         = 118
	ShadowlordBoundary8                         = 119
	ShadowlordBoundary9                         = 120
	ShadowlordBoundary10                        = 121
	ShadowlordBoundary11                        = 122
	ShadowlordBoundary12                        = 123
	ShadowlordBoundary13                        = 124
	ShadowlordBoundary14                        = 125
	ShadowlordBoundary15                        = 126
	ShadowlordBoundary16                        = 127
	TortureChair1                               = 128
	TortureTable2                               = 129
	TortureTableWithBody1                       = 130
	TortureTableWithBody2                       = 131
	Stocks                                      = 132
	Manacles                                    = 133
	Grate                                       = 134
	BrickWallArchway                            = 135
	CannonBalls                                 = 136
	SimpleCross                                 = 137
	StoneHeadstone                              = 138
	TortureChair                                = 139
	BrickFloorHole                              = 140
	Harpsichord                                 = 141
	Guillotine                                  = 142
	Lava                                        = 143
	ChairBackForward                            = 144
	ChairBackLeft                               = 145
	ChairBackBack                               = 146
	ChairBackRight                              = 147
	TableLeft                                   = 148
	TableMiddle                                 = 149
	TableRight                                  = 150
	MagicLockDoor                               = 151
	MagicLockDoorWithView                       = 152
	Portcullis                                  = 153
	TableFoodTop                                = 154
	TableFoodBottom                             = 155
	TableFoodBoth                               = 156
	Mirror                                      = 157
	MirrorAvatar                                = 158
	MirrorBroken                                = 159
	Sign                                        = 160
	Well                                        = 161
	HitchingPost                                = 162
	FireLogs                                    = 163
	SmallSign                                   = 164
	EndTable                                    = 165
	Barrel                                      = 166
	Cask                                        = 167
	Vanity                                      = 168
	WaterJugTable                               = 169
	Carpet                                      = 170
	LeftBed                                     = 171
	RightBed                                    = 172
	Dresser                                     = 173
	CoffeeTable                                 = 174
	Box                                         = 175
	RightSconce                                 = 176
	LeftSconce                                  = 177
	Brazier                                     = 178
	CampFire                                    = 179
	CannonUp                                    = 180
	CannonRight                                 = 181
	CannonDown                                  = 182
	CannonLeft                                  = 183
	RegularDoor                                 = 184
	LockedDoor                                  = 185
	RegularDoorView                             = 186
	LockedDoorView                              = 187
	Fireplace                                   = 188
	LampPost                                    = 189
	CandleOnTable                               = 190
	CookStove                                   = 191
	ScaryBlackThing1                            = 192
	ScaryBlackThing2                            = 193
	ScaryBlackThing3                            = 194
	ScaryBlackThingDead                         = 195
	StairsNorth                                 = 196
	StairsEast                                  = 197
	StairsSouth                                 = 198
	StairsWest                                  = 199
	LadderUp                                    = 200
	LadderDown                                  = 201
	FenceHoriz                                  = 202
	FenceVert                                   = 203
	Wtf1                                        = 204
	Wtf2                                        = 205
	Wtf3                                        = 206
	Wtf4                                        = 207
	CornerStructure1                            = 208
	CornerStructure2                            = 209
	CornerStructure3                            = 210
	CornerStructure4                            = 211
	Waterfall1                                  = 212
	Waterfall2                                  = 213
	Waterfall3                                  = 214
	Waterfall4                                  = 215
	Fountain1                                   = 216
	Fountain2                                   = 217
	Fountain3                                   = 218
	Fountain4                                   = 219
	Moongate                                    = 220
	Desert3                                     = 221
	BlueFlame                                   = 222
	BlockEntrance                               = 223
	LargeSignBottom                             = 224
	LargeSignLeft                               = 225
	LargeSignRight                              = 226
	SignGrass                                   = 227
	CornerWithWater1                            = 228
	CornerWithWater2                            = 229
	CornerWithWater3                            = 230
	CornerWithWater4                            = 231
	Hourglass1                                  = 232
	Hourglass2                                  = 233
	Hourglass3                                  = 234
	Hourglass4                                  = 235
	SnakeSign1                                  = 236
	SnakeSign2                                  = 237
	SnakeSign3                                  = 238
	SnakeSign4                                  = 239
	SignFood                                    = 240
	SignCourt                                   = 241
	SignArmourer                                = 242
	SignHealer                                  = 243
	SignSmith                                   = 244
	SignGuild                                   = 245
	SignInn                                     = 246
	SignMagic                                   = 247
	SignWarning                                 = 248
	SignShipwright                              = 249
	Clock1                                      = 250
	Clock2                                      = 251
	Bellows1                                    = 252
	Bellows2                                    = 253
	WhiteSquare                                 = 254
	BlackSquare                                 = 255
	StarPattern                                 = 256
	Chest                                       = 257
	ItemMoney                                   = 258
	ItemPotion                                  = 259
	ItemScroll                                  = 260
	ItemWeapon                                  = 261
	ItemShield                                  = 262
	ItemKey                                     = 263
	ItemGem                                     = 264
	ItemHelm                                    = 265
	ItemRing                                    = 266
	ItemArmour                                  = 267
	ItemAnkh                                    = 268
	ItemTorch                                   = 269
	ItemSandalwoodBox                           = 270
	ItemFood                                    = 271
	HorseRight                                  = 272
	HorseLeft                                   = 273
	RidingHorseRight                            = 274
	RidingHorseLeft                             = 275
	RidingMagicCarpetRight                      = 276
	RidingMagicCarpetLeft                       = 277
	HolyFloorSymbol                             = 278
	KlimbLadderUp                               = 279
	KlimbLadderDown                             = 280
	Moon                                        = 281
	SleepingInBed                               = 282
	Carpet2                                     = 283
	BasicAvatar                                 = 284
	InvisiblePlayer                             = 285 // Original INVIS_PC (0x1d + tile bank offset)
	DeadBody                                    = 286
	Splat                                       = 287
	ShipSailsUp                                 = 288
	ShipSailsRight                              = 289
	ShipSailsDown                               = 290
	ShipSailsLeft                               = 291
	ShipNoSailsUp                               = 292
	ShipNoSailsRight                            = 293
	ShipNoSailsDown                             = 294
	ShipNoSailsLeft                             = 295
	SkiffUp                                     = 296
	SkiffRight                                  = 297
	SkiffDown                                   = 298
	SkiffLeft                                   = 299
	PirateShipUp                                = 300
	PirateShipRight                             = 301
	PirateShipDown                              = 302
	PirateShipLeft                              = 303
	SitChairUp                                  = 304
	SitChairRight                               = 305
	SitChairDown                                = 306
	SitChairLeft                                = 307
	SitChairDownEat1                            = 308
	SitChairDownEat2                            = 309
	SitChairDownEat3                            = 310
	SitChairDownEat4                            = 311
	SitChairUpEat1                              = 312
	SitChairUpEat2                              = 313
	SitChairUpEat3                              = 314
	SitChairUpEat4                              = 315
	LordBritishMirror1                          = 316
	LordBritishMirror2                          = 317
	LordBritishMirror3                          = 318
	LordBritishMirror4                          = 319
	Wizard1                                     = 320
	Wizard2                                     = 321
	Wizard3                                     = 322
	Wizard4                                     = 323
	Bard1                                       = 324
	Bard2                                       = 325
	Bard3                                       = 326
	Bard4                                       = 327
	Fighter1                                    = 328
	Fighter2                                    = 329
	Fighter3                                    = 330
	Fighter4                                    = 331
	Avatar1                                     = 332
	Avatar2                                     = 333
	Avatar3                                     = 334
	Avatar4                                     = 335
	TownsPerson1                                = 336
	TownsPerson2                                = 337
	TownsPerson3                                = 338
	TownsPerson4                                = 339
	Merchant1                                   = 340
	Merchant2                                   = 341
	Merchant3                                   = 342
	Merchant4                                   = 343
	Jester1                                     = 344
	Jester2                                     = 345
	Jester3                                     = 346
	Jester4                                     = 347
	BardPlaying1                                = 348
	BardPlaying2                                = 349
	BardPlaying3                                = 350
	BardPlaying4                                = 351
	PersonStocks1                               = 352
	PersonStocks2                               = 353
	PersonStocks3                               = 354
	PersonStocks4                               = 355
	WallPrisoner1                               = 356
	WallPrisoner2                               = 357
	WallPrisoner3                               = 358
	WallPrisoner4                               = 359
	Child1                                      = 360
	Child2                                      = 361
	Child3                                      = 362
	Child4                                      = 363
	Begger1                                     = 364
	Begger2                                     = 365
	Begger3                                     = 366
	Begger4                                     = 367
	Guard1                                      = 368
	Guard2                                      = 369
	Guard3                                      = 370
	Guard4                                      = 371
	Apparation1                                 = 372
	Apparation2                                 = 373
	Apparation3                                 = 374
	Apparation4                                 = 375
	Blackthorn1                                 = 376
	Blackthorn2                                 = 377
	Blackthorn3                                 = 378
	Blackthorn4                                 = 379
	LordBritish1                                = 380
	LordBritish2                                = 381
	LordBritish3                                = 382
	LordBritish4                                = 383
	Seahorse1                                   = 384
	Seahorse2                                   = 385
	Seahorse3                                   = 386
	Seahorse4                                   = 387
	Squid1                                      = 388
	Squid2                                      = 389
	Squid3                                      = 390
	Squid4                                      = 391
	SeaSerpent1                                 = 392
	SeaSerpent2                                 = 393
	SeaSerpent3                                 = 394
	SeaSerpent4                                 = 395
	Shark1                                      = 396
	Shark2                                      = 397
	Shark3                                      = 398
	Shark4                                      = 399
	Rat1                                        = 400
	Rat2                                        = 401
	Rat3                                        = 402
	Rat4                                        = 403
	Bat1                                        = 404
	Bat2                                        = 405
	Bat3                                        = 406
	Bat4                                        = 407
	Spider1                                     = 408
	Spider2                                     = 409
	Spider3                                     = 410
	Spider4                                     = 411
	Ghost1                                      = 412
	Ghost2                                      = 413
	Ghost3                                      = 414
	Ghost4                                      = 415
	Slime1                                      = 416
	Slime2                                      = 417
	Slime3                                      = 418
	Slime4                                      = 419
	Gremlin1                                    = 420
	Gremlin2                                    = 421
	Gremlin3                                    = 422
	Gremlin4                                    = 423
	Mimic1                                      = 424
	Mimic2                                      = 425
	Mimic3                                      = 426
	Mimic4                                      = 427
	Reaper1                                     = 428
	Reaper2                                     = 429
	Reaper3                                     = 430
	Reaper4                                     = 431
	Gazer1                                      = 432
	Gazer2                                      = 433
	Gazer3                                      = 434
	Gazer4                                      = 435
	Shard                                       = 436
	Crown                                       = 437
	Sceptre                                     = 438
	Amulet                                      = 439
	BlackBadge                                  = StarPattern // Uses generic star icon
	StoneGargoyle1                              = 440
	StoneGargoyle2                              = 441
	StoneGargoyle3                              = 442
	StoneGargoyle4                              = 443
	InsectSwarm1                                = 444
	InsectSwarm2                                = 445
	InsectSwarm3                                = 446
	InsectSwarm4                                = 447
	Orc1                                        = 448
	Orc2                                        = 449
	Orc3                                        = 450
	Orc4                                        = 451
	Skeleton1                                   = 452
	Skeleton2                                   = 453
	Skeleton3                                   = 454
	Skeleton4                                   = 455
	Snake1                                      = 456
	Snake2                                      = 457
	Snake3                                      = 458
	Snake4                                      = 459
	Ettin1                                      = 460
	Ettin2                                      = 461
	Ettin3                                      = 462
	Ettin4                                      = 463
	Headless1                                   = 464
	Headless2                                   = 465
	Headless3                                   = 466
	Headless4                                   = 467
	Wisp1                                       = 468
	Wisp2                                       = 469
	Wisp3                                       = 470
	Wisp4                                       = 471
	Daemon1                                     = 472
	Daemon2                                     = 473
	Daemon3                                     = 474
	Daemon4                                     = 475
	Dragon1                                     = 476
	Dragon2                                     = 477
	Dragon3                                     = 478
	Dragon4                                     = 479
	SandTrap1                                   = 480
	SandTrap2                                   = 481
	SandTrap3                                   = 482
	SandTrap4                                   = 483
	Troll1                                      = 484
	Troll2                                      = 485
	Troll3                                      = 486
	Troll4                                      = 487
	PoisonField                                 = 488
	MagicField                                  = 489
	FireField                                   = 490
	ElectricField                               = 491
	Whirpool1                                   = 492
	Whirpool2                                   = 493
	Whirpool3                                   = 494
	Whirpool4                                   = 495
	MongBat1                                    = 496
	MongBat2                                    = 497
	MongBat3                                    = 498
	MongBat4                                    = 499
	Corpser1                                    = 500
	Corpser2                                    = 501
	Corpser3                                    = 502
	Corpser4                                    = 503
	RotWork1                                    = 504
	RotWork2                                    = 505
	RotWork3                                    = 506
	RotWork4                                    = 507
	ShadowLord1                                 = 508
	ShadowLord2                                 = 509
	ShadowLord3                                 = 510
	ShadowLord4                                 = 511
	BrickWallArchwayWithPortcullis              = 512
	RidingHorseDown                             = 513
	RidingHorseUp                               = 514
	RidingMagicCarpetDown                       = 515
	RidingMagicCarpetUp                         = 516
	CombatHighlightBox                          = 517
	FullCombatHighlightBox                      = 518
	HittableCombatHighlightBox                  = 519
	NotHittableCombatHighlightBox               = 520
	HittableRangeCombatHighlightBox             = 521
	Arrow                                       = 522
	Axe                                         = 523
	SlingRock                                   = 524
	CannonBall                                  = 525
	HitBang                                     = 526
	MissedBang                                  = 527
	GrazedBang                                  = 528
)

// Animation key indices mark the first frame of each animation sequence; the
// following frames are the next StandardNumberOfAnimationFrames sprites.
const (
	MageKeyIndex          SpriteIndex = 320 // Same as Wizard1
	WizardKeyIndex        SpriteIndex = 320 // Same as Wizard1
	BardKeyIndex          SpriteIndex = 324 // Bard1
	FighterKeyIndex       SpriteIndex = 328 // Fighter1
	AvatarKeyIndex        SpriteIndex = 332 // Avatar1
	TownsPersonKeyIndex   SpriteIndex = 336 // TownsPerson1
	MerchantKeyIndex      SpriteIndex = 340 // Merchant1
	BardPlayingKeyIndex   SpriteIndex = 348 // BardPlaying1
	BeggarKeyIndex        SpriteIndex = 365 // Begger2 (note: CSV has typo)
	GuardKeyIndex         SpriteIndex = 368 // Guard1
	RatKeyIndex           SpriteIndex = 400 // Rat1
	BatKeyIndex           SpriteIndex = 404 // Bat1
	SpiderKeyIndex        SpriteIndex = 408 // Spider1
	GhostKeyIndex         SpriteIndex = 412 // Ghost1
	SlimeKeyIndex         SpriteIndex = 416 // Slime1
	ReaperKeyIndex        SpriteIndex = 420 // Gremlin1
	GazerKeyIndex         SpriteIndex = 424 // Mimic1
	OrcKeyIndex           SpriteIndex = 432 // Gazer1
	SkeletonKeyIndex      SpriteIndex = 452 // Skeleton1
	StoneGargoyleKeyIndex SpriteIndex = 440 // StoneGargoyle1
	SnakeKeyIndex         SpriteIndex = 456 // Snake1
	DragonKeyIndex        SpriteIndex = 476 // Dragon1
	DaemonKeyIndex        SpriteIndex = 472 // Daemon1
	TrollKeyIndex         SpriteIndex = 484 // Troll1
	WhirlpoolKeyIndex     SpriteIndex = 492 // Whirpool1
	SandtrapKeyIndex      SpriteIndex = 480 // SandTrap1
	ShadowLordKeyIndex    SpriteIndex = 508 // ShadowLord1
	WaterfallKeyIndex     SpriteIndex = 212 // Waterfall1
	FountainKeyIndex      SpriteIndex = 216 // Fountain1
	ManaclesPrisoner      SpriteIndex = 356 // WallPrisoner1
)

// IsDoor reports whether s is any door tile: plain, windowed or magic-locked.
func (s SpriteIndex) IsDoor() bool {
	return s == RegularDoor || s == LockedDoor || s == RegularDoorView ||
		s == LockedDoorView || s == MagicLockDoorWithView || s == MagicLockDoor
}

// IsUnlockedDoor reports whether s is a regular door, with or without a view
// window, rather than a locked one.
func (s SpriteIndex) IsUnlockedDoor() bool {
	return s == RegularDoor || s == RegularDoorView
}

// IsWindowedDoor reports whether s is a door with a view window.
func (s SpriteIndex) IsWindowedDoor() bool {
	return s == RegularDoorView || s == LockedDoorView || s == MagicLockDoorWithView
}

// IsBed reports whether s is a bed tile, including the occupied bed.
func (s SpriteIndex) IsBed() bool {
	return s == LeftBed || s == SleepingInBed || s == RightBed
}

// IsStairs reports whether s is a staircase tile facing any direction.
func (s SpriteIndex) IsStairs() bool {
	return s == StairsSouth || s == StairsWest || s == StairsNorth || s == StairsEast
}

// IsFrigateFurled reports whether s is a frigate with its sails furled.
func (s SpriteIndex) IsFrigateFurled() bool {
	return s == ShipNoSailsLeft || s == ShipNoSailsRight || s == ShipNoSailsDown || s == ShipNoSailsUp
}

// IsHorseUnBoarded reports whether s is a horse with no rider.
func (s SpriteIndex) IsHorseUnBoarded() bool {
	return s == HorseLeft || s == HorseRight
}

// IsSkiff reports whether s is a skiff facing any direction.
func (s SpriteIndex) IsSkiff() bool {
	return s == SkiffDown || s == SkiffLeft || s == SkiffRight || s == SkiffUp
}

// IsMagicCarpetUnboarded reports whether s is the magic carpet with no rider.
func (s SpriteIndex) IsMagicCarpetUnboarded() bool {
	return s == Carpet2
}

// IsPartOfAnimation reports whether s belongs to the animation that starts at
// keyFrameIndex: that sprite and the next StandardNumberOfAnimationFrames-1.
// Sprites below 304 are not animated and match only keyFrameIndex itself.
func (s SpriteIndex) IsPartOfAnimation(keyFrameIndex SpriteIndex) bool {
	const lowestPossibleAnimationIndex = 304
	if s < lowestPossibleAnimationIndex {
		return s == keyFrameIndex
	}

	return s >= keyFrameIndex && s < keyFrameIndex+StandardNumberOfAnimationFrames
}

// IsBeggar returns true if the sprite is part of the beggar animation sequence.
// Beggar sprites are indices 364-367 (Begger1-Begger4).
// Used for karma bonus detection when giving gold to beggars in conversations.
// The Beggar family occupies raw tiles 0x6c..0x6f; ref:ENEMY-0001.
func (s SpriteIndex) IsBeggar() bool {
	return s >= Begger1 && s <= Begger4
}

// IsSign reports whether this terrain selects sign inspection; ref:SIGN-0003.
func (s SpriteIndex) IsSign() bool {
	return s == SimpleCross || s == StoneHeadstone || s == Sign || s == SmallSign || s == SignWarning
}

// IsBardPlaying returns true if the sprite is part of the BANJO_PLAYER animation sequence.
// BardPlaying sprites are indices 348-351 (BardPlaying1-BardPlaying4).
// Used for sprite switching logic: when a BANJO_PLAYER NPC is seated on a south-facing chair,
// they display the BardPlaying sprite (playing music); otherwise they convert to Merchant.
// Bard presentation switches the seated musician's sprite; ref:TILE-0007.
func (s SpriteIndex) IsBardPlaying() bool {
	return s >= BardPlaying1 && s <= BardPlaying4
}
