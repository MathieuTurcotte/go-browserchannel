goog.provide('tests.Handler1');

goog.require('goog.Timer');
goog.require('goog.debug.Logger');
goog.require('goog.net.BrowserChannel.Handler');

tests.Handler1 = function() {
    goog.base(this);

    this.timer_ = new goog.Timer(tests.Handler1.TIMEOUT_DELAY);

    this.eventHandler_ = new goog.events.EventHandler(this);
    this.eventHandler_.listen(this.timer_, goog.Timer.TICK, this.onTimeout_);
};
goog.inherits(tests.Handler1, goog.net.BrowserChannel.Handler);

tests.Handler1.TIMEOUT_DELAY = 25 * 1000;

tests.Handler1.prototype.logger =
    goog.debug.Logger.getLogger('tests.Handler1');

tests.Handler1.prototype.channelOpened = function(channel) {
    this.logger.info('channelOpened');
    this.timer_.start();
};

tests.Handler1.prototype.channelHandleArray = function(channel, array) {
    this.logger.info('channelHandleArray: ' + goog.debug.expose(array));
    this.timer_.setInterval(tests.Handler1.TIMEOUT_DELAY);
};

tests.Handler1.prototype.channelError = function(channel, error) {
    switch (error) {
        case goog.net.BrowserChannel.Error.STOP:
            break;
        default:
            this.logger.info('channelError: ' + error);
            window.callPhantom({success: false});
    }
};

tests.Handler1.prototype.channelClosed = function(channel) {
    this.logger.info('channelClosed');
    window.callPhantom({success: true});
};

tests.Handler1.prototype.badMapError = function(browserChannel, map) {
    this.logger.info('badMapError: ' + goog.debug.expose(map));
    window.callPhantom({success: false});
};

tests.Handler1.prototype.onTimeout_ = function(channel) {
    this.logger.info('timeout');
    channel.disconnect();
    window.callPhantom({success: false});
};
