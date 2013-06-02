/*
 * Copyright (c) 2013 Mathieu Turcotte
 * Licensed under the MIT license.
 */

goog.provide('tests.Handler2');

goog.require('goog.Timer');
goog.require('goog.debug.Logger');
goog.require('goog.net.BrowserChannel.Handler');



/**
 * A test handler which expects the server to send arrays every 15 seconds and
 * then to close the channel.
 * @extends {goog.net.BrowserChannel.Handler}
 * @constructor
 */
tests.Handler2 = function() {
    goog.base(this);

    this.timer_ = new goog.Timer(tests.Handler2.TIMEOUT_DELAY);

    this.eventHandler_ = new goog.events.EventHandler(this);
    this.eventHandler_.listen(this.timer_, goog.Timer.TICK, this.onTimeout_);
};
goog.inherits(tests.Handler2, goog.net.BrowserChannel.Handler);


/**
 * Maximum delay to wait for an array from the server. If the delay is
 * exceeded, the handler will signal a failure.
 * @type {number}
 * @const
 */
tests.Handler2.TIMEOUT_DELAY = 25 * 1000;


/**
 * @private {!goog.debug.Logger}
 */
tests.Handler2.prototype.logger_ =
    goog.debug.Logger.getLogger('tests.Handler2');


/** @override */
tests.Handler2.prototype.channelOpened = function(channel) {
    this.logger_.info('channelOpened');
    channel.sendMap({id: '2'});
    this.timer_.start();
};


/** @override */
tests.Handler2.prototype.channelHandleArray = function(channel, array) {
    this.logger_.info('channelHandleArray: ' + goog.debug.expose(array));
    this.timer_.setInterval(tests.Handler2.TIMEOUT_DELAY);
};


/** @override */
tests.Handler2.prototype.channelError = function(channel, error) {
    switch (error) {
        case goog.net.BrowserChannel.Error.STOP:
            break;
        default:
            this.logger_.info('channelError: ' + error);
            window.callPhantom({type: 'done', success: false});
    }
};


/** @override */
tests.Handler2.prototype.channelClosed = function(channel) {
    this.logger_.info('channelClosed');
    window.callPhantom({type: 'done', success: true});
};


/** @override */
tests.Handler2.prototype.badMapError = function(browserChannel, map) {
    this.logger_.info('badMapError: ' + goog.debug.expose(map));
    window.callPhantom({type: 'done', success: false});
};


/**
 * Handles the timeout triggered if no arrays were received within the
 * expected delay from the server.
 * @private
 */
tests.Handler2.prototype.onTimeout_ = function() {
    this.logger_.info('timeout');
    window.callPhantom({type: 'done', success: false});
};
