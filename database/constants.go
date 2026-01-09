package database

import "time"

// Time interval constants for TimescaleDB operations
const (
	// Continuous aggregate refresh intervals
	RefreshInterval1Min  = 1 * time.Minute
	RefreshInterval5Min  = 5 * time.Minute
	RefreshInterval15Min = 15 * time.Minute
	RefreshInterval1Hour = 1 * time.Hour
	RefreshInterval1Day  = 24 * time.Hour

	// Continuous aggregate start offsets
	StartOffset1Min  = 3 * time.Minute
	StartOffset5Min  = 15 * time.Minute
	StartOffset15Min = 45 * time.Minute
	StartOffset1Hour = 3 * time.Hour
	StartOffset1Day  = 3 * 24 * time.Hour

	// Continuous aggregate end offsets
	EndOffset1Min  = 1 * time.Minute
	EndOffset5Min  = 1 * time.Minute
	EndOffset15Min = 1 * time.Minute
	EndOffset1Hour = 1 * time.Minute
	EndOffset1Day  = 1 * time.Hour

	// Hypertable chunk intervals
	ChunkInterval1Day  = 1 * 24 * time.Hour
	ChunkInterval7Days = 7 * 24 * time.Hour

	// Data retention policies
	Retention3Months = 3 * 30 * 24 * time.Hour
	Retention6Months = 6 * 30 * 24 * time.Hour
	Retention1Year   = 365 * 24 * time.Hour
	Retention2Years  = 2 * 365 * 24 * time.Hour
	Retention10Years = 10 * 365 * 24 * time.Hour
)

// Z-score thresholds for statistical analysis
const (
	// Volume analysis thresholds
	ZScoreVolumeLow      = 1.0
	ZScoreVolumeModerate = 2.0
	ZScoreVolumeHigh     = 3.0
	ZScoreVolumeExtreme  = 6.0

	// Price analysis thresholds
	ZScorePriceBreakout = 2.0
	ZScorePriceHigh     = 4.0
	ZScorePriceExtreme  = 7.0

	// Anomaly detection thresholds
	ZScoreAnomalyMin = 3.0
)

// Percentage thresholds for trading signals
const (
	// Price change thresholds
	PriceChangeThresholdBreakout = 2.0
	PriceChangeThresholdHigh     = 3.0

	// Accumulation/Distribution thresholds
	AccumulationThreshold = 55.0
	DistributionThreshold = 55.0
)

// Confidence score thresholds
const (
	ConfidenceVeryLow  = 0.1
	ConfidenceLow      = 0.3
	ConfidenceMedium   = 0.4
	ConfidenceHigh     = 0.45
	ConfidenceVeryHigh = 0.6
	ConfidenceMax      = 0.8
)

// Lookback periods for analysis
const (
	LookbackMinutesDefault = 60
	LookbackMinutesShort   = 30
	LookbackMinutesLong    = 120
	LookbackHoursDefault   = 24
	LookbackHoursShort     = 2
	LookbackDaysDefault    = 7
)

// Query limits
const (
	DefaultLimit      = 50
	TopLimit          = 20
	MaxLimit          = 100
	MinSampleSize     = 10
	MinSampleSizeHigh = 30
)

// Strategy-specific constants
const (
	// Volume Breakout Strategy
	VolumeBreakoutMinPriceChange  = 2.0
	VolumeBreakoutMinVolumeZScore = 2.5
	VolumeBreakoutMaxVolumeZScore = 6.0

	// Mean Reversion Strategy
	MeanReversionMinPriceZScore = 3.0
	MeanReversionMaxPriceZScore = 7.0

	// Fakeout Filter Strategy
	FakeoutFilterMinPriceChange    = 3.0
	FakeoutFilterMinVolumeZScore   = 1.0
	FakeoutFilterValidVolumeZScore = 2.0
	FakeoutFilterMaxVolumeZScore   = 5.0
)

// Market regime confidence thresholds
const (
	RegimeConfidenceLow  = 0.5
	RegimeConfidenceHigh = 0.6
)

// Pattern detection constants
const (
	PatternLookbackHours     = 2
	PatternConfirmationBoost = 1.3
	PatternFilterPenalty     = 0.5
	PatternRegimeBoost       = 1.2
)

// Whale alert constants
const (
	MinAlertsForPattern     = 3
	WhaleAlertLookbackHours = 24
)

// Signal tracking constants
const (
	SignalLookbackMinutes = 60
	MaxSignalsPerQuery    = 50
)

// Followup tracking constants
const (
	FollowupMaxAge = 24 * time.Hour
)

// Performance tracking constants
const (
	PerformanceLookbackDays = 30
)
